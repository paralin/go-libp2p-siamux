package siamux

import (
	"bytes"
	"context"
	"net"
	"sync/atomic"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/pkg/errors"
	smux "go.sia.tech/mux"
)

// openStreamMsg is the message sent to open the stream.
var openStreamMsg = []byte{42}

// MuxedConn implements the libp2p MuxedConn interface with the Sia muxer.
//
// To maintain semantics from the MuxedConn in core/network, we send a 1-byte
// "stream open" packet to force sia mux to open / ack the stream immediately.
type MuxedConn struct {
	// establishCh contains the result of the Establish process.
	establishCh chan error
	// mux is the sia muxer
	// nil until establishCh contains nil
	mux *smux.Mux
	// closed indicates Close has been called
	closed atomic.Bool
}

// NewMuxedConn constructs a new MuxedConn from a smux.Mux.
//
// To maintain the same semantics as yamux.NewMuxedConn, NewMuxedConn returns
// immediately, starting a new goroutine to dial or accept the connection.
//
// outbound indicates if this is a client or server side.
// uses the Anonymous mode of the muxer.
func NewMuxedConn(conn net.Conn, outbound bool) (*MuxedConn, error) {
	mc := &MuxedConn{establishCh: make(chan error, 1)}
	go mc.establish(conn, outbound)
	return mc, nil
}

// EstablishMux establishes a muxed conn waiting for it to be confirmed before returning.
func EstablishMux(conn net.Conn, outbound bool) (*smux.Mux, error) {
	if outbound {
		return smux.DialAnonymous(conn)
	} else {
		return smux.AcceptAnonymous(conn)
	}
}

// EstablishMuxedConn waits for the muxed conn to be fully established before returning.
//
// outbound indicates if this is a client or server side.
// uses the Anonymous mode of the muxer.
func EstablishMuxedConn(conn net.Conn, outbound bool) (*MuxedConn, error) {
	mux, err := EstablishMux(conn, outbound)
	if err != nil {
		return nil, err
	}
	return WrapMux(mux), nil
}

// WrapMux wraps an existing Mux into a MuxedConn.
func WrapMux(mux *smux.Mux) *MuxedConn {
	establishCh := make(chan error, 1)
	establishCh <- nil
	return &MuxedConn{mux: mux, establishCh: establishCh}
}

// WaitEstablished waits for the Dial or Accept goroutine to complete.
func (m *MuxedConn) WaitEstablished(ctx context.Context) (*smux.Mux, error) {
	select {
	case <-ctx.Done():
		return nil, context.Canceled
	case err := <-m.establishCh:
		select {
		case m.establishCh <- err:
		default:
		}
		return m.mux, err
	}
}

// establish is the goroutine to establish the muxed conn.
func (m *MuxedConn) establish(conn net.Conn, outbound bool) {
	mux, err := EstablishMux(conn, outbound)
	m.mux = mux
	select {
	case m.establishCh <- err:
	default:
	}
}

// waitEstablish waits for the goroutine to establish the muxed conn.
func (m *MuxedConn) waitEstablish() (*smux.Mux, error) {
	err := <-m.establishCh
	select {
	case m.establishCh <- err:
	default:
	}
	return m.mux, err
}

// IsClosed returns whether a connection is fully closed, so it can be garbage
// collected.
func (m *MuxedConn) IsClosed() bool {
	return m.closed.Load()
}

// Close closes the conn.
func (m *MuxedConn) Close() error {
	defer m.closed.Store(true)
	mux, err := m.waitEstablish()
	if err != nil {
		// ignore establish error in Close()
		return nil
	}
	return mux.Close()
}

// OpenStream creates a new stream.
func (m *MuxedConn) OpenStream(ctx context.Context) (network.MuxedStream, error) {
	mux, err := m.WaitEstablished(ctx)
	if err != nil {
		return nil, err
	}
	strm := mux.DialStream()
	_, err = strm.Write(openStreamMsg)
	if err != nil {
		return nil, err
	}
	return NewMuxedStream(strm), nil
}

// AcceptStream accepts a stream opened by the other side.
func (m *MuxedConn) AcceptStream() (network.MuxedStream, error) {
	mux, err := m.waitEstablish()
	if err != nil {
		return nil, err
	}
	strm, err := mux.AcceptStream()
	if err != nil {
		return nil, err
	}

	rxBuf := make([]byte, 1)
	for {
		n, err := strm.Read(rxBuf)
		if err != nil {
			return nil, err
		}
		if n != 0 {
			if !bytes.Equal(rxBuf, openStreamMsg) {
				return nil, errors.Errorf("siamux: expected open stream pkt %v but got %v", openStreamMsg, rxBuf)
			}
			break
		}
	}

	return NewMuxedStream(strm), nil
}

// _ is a type assertion
var _ network.MuxedConn = ((*MuxedConn)(nil))
