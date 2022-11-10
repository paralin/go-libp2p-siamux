package siamux

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
)

// TestSiaMux tests the sia muxer.
func TestSiaMux(t *testing.T) {
	serverConn, clientConn := net.Pipe()

	errCh := make(chan error, 1)
	go func() {
		mc, err := NewMuxedConn(serverConn, false)
		if err != nil {
			errCh <- err
			return
		}

		strm, err := mc.AcceptStream()
		if err != nil {
			errCh <- err
			return
		}

		_, err = io.Copy(strm, bytes.NewBuffer([]byte("hello world from server")))
		if err != nil {
			errCh <- err
			return
		}
		errCh <- strm.Close()
	}()

	cc, err := NewMuxedConn(clientConn, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	st, err := cc.OpenStream(context.Background())
	if err != nil {
		t.Fatal(err.Error())
	}

	// we have to write something to open the stream
	_, err = st.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err.Error())
	}

	data, err := io.ReadAll(st)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("read data: %s\n", string(data))
	if err := st.Close(); err != nil {
		t.Fatal(err.Error())
	}
}

// TestSiaMux_SRPC tests SiaMux with Starpc.
func TestSiaMux_SRPC(t *testing.T) {
	// construct the server
	mux := srpc.NewMux()
	echoServer := echo.NewEchoServer(mux)
	if err := echo.SRPCRegisterEchoer(mux, echoServer); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)

	clientPipe, serverPipe := net.Pipe()

	// outbound=false
	serverMp, err := NewMuxedConn(serverPipe, false)
	if err != nil {
		t.Fatalf(err.Error())
	}

	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() {
		if err := server.AcceptMuxedConn(ctx, serverMp); err != nil {
			t.Logf("error accepting muxed conn streams: %s", err.Error())
			errCh <- err
		}
	}()

	// outbound=true
	clientMp, err := NewMuxedConn(clientPipe, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	client := srpc.NewClientWithMuxedConn(clientMp)

	// construct the client rpc interface
	clientEcho := echo.NewSRPCEchoerClient(client)

	// call echo
	expected := "hello world"
	msg, err := clientEcho.Echo(ctx, &echo.EchoMsg{Body: expected})
	if err != nil {
		t.Fatal(err.Error())
	}

	if msg.GetBody() != expected {
		t.Fatalf("expected %q but got %q", expected, msg.GetBody())
	}
}
