package siamux

import (
	"io"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/network"

	smux "go.sia.tech/mux"
)

// MuxedStream implements network.MuxedStream with a siamux Stream.
type MuxedStream struct {
	strm *smux.Stream

	writeClosed atomic.Bool
	readClosed  atomic.Bool
}

// NewMuxedStream constructs a new MuxedStream.
func NewMuxedStream(strm *smux.Stream) *MuxedStream {
	return &MuxedStream{strm: strm}
}

// CloseWrite closes the stream for writing but leaves it open for reading.
func (s *MuxedStream) CloseWrite() error {
	s.writeClosed.Store(true)
	return nil
}

// CloseRead closes the stream for reading but leaves it open for writing.
func (s *MuxedStream) CloseRead() error {
	s.readClosed.Store(true)
	return nil
}

// Read reads data from the stream.
func (s *MuxedStream) Read(p []byte) (n int, err error) {
	if s.readClosed.Load() {
		return 0, io.ErrClosedPipe
	}
	return s.strm.Read(p)
}

// Write writes data to the stream.
func (s *MuxedStream) Write(p []byte) (n int, err error) {
	if s.writeClosed.Load() {
		return 0, io.ErrClosedPipe
	}
	return s.strm.Write(p)
}

func (s *MuxedStream) SetDeadline(t time.Time) error {
	return s.strm.SetDeadline(t)
}

func (s *MuxedStream) SetReadDeadline(t time.Time) error {
	return s.strm.SetReadDeadline(t)
}

func (s *MuxedStream) SetWriteDeadline(t time.Time) error {
	return s.strm.SetWriteDeadline(t)
}

// Reset closes both ends of the stream. Use this to tell the remote
// side to hang up and go away.
func (s *MuxedStream) Reset() error {
	return s.Close()
}

// Close closes the stream.
func (s *MuxedStream) Close() error {
	s.writeClosed.Store(true)
	s.readClosed.Store(true)
	return s.strm.Close()
}

// _ is a type assertion
var _ network.MuxedStream = ((*MuxedStream)(nil))
