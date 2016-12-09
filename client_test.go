package fastrpc

import (
	"bufio"
	"fmt"
	"github.com/valyala/fasthttp/fasthttputil"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

type testRequest struct {
	b       []byte
	sizeBuf [4]byte
}

func (r *testRequest) WriteRequest(bw *bufio.Writer) error {
	err := testWriteMessage(bw, r.b, r.sizeBuf[:])
	if err != nil {
		err = fmt.Errorf("cannot write request: %s", err)
	}
	return err
}

func (r *testRequest) ReadRequest(br *bufio.Reader) error {
	var err error
	r.b, err = testReadMessage(br, r.b, r.sizeBuf[:])
	if err != nil {
		err = fmt.Errorf("cannot read request: %s", err)
	}
	return err
}

type testResponse struct {
	b       []byte
	sizeBuf [4]byte
}

func (r *testResponse) WriteResponse(bw *bufio.Writer) error {
	err := testWriteMessage(bw, r.b, r.sizeBuf[:])
	if err != nil {
		err = fmt.Errorf("cannot write response: %s", err)
	}
	return err
}

func (r *testResponse) ReadResponse(br *bufio.Reader) error {
	var err error
	r.b, err = testReadMessage(br, r.b, r.sizeBuf[:])
	if err != nil {
		err = fmt.Errorf("cannot read response: %s", err)
	}
	return err
}

func testWriteMessage(bw *bufio.Writer, msg, sizeBuf []byte) error {
	msgSize := len(msg)
	sizeBuf = appendUint32(sizeBuf[:0], uint32(msgSize))
	_, err := bw.Write(sizeBuf)
	if err == nil {
		_, err = bw.Write(msg)
	}
	return err
}

func testReadMessage(br *bufio.Reader, msg, sizeBuf []byte) ([]byte, error) {
	n, err := io.ReadFull(br, sizeBuf)
	if err != nil {
		return msg, fmt.Errorf("cannot read message size: %s", err)
	}
	if n != 4 {
		return msg, fmt.Errorf("unexpected number of bytes read when reading message size: %d. Expecting 4", n)
	}
	var b [4]byte
	copy(b[:], sizeBuf)
	msgSize := int(bytes2Uint32(b))
	if msgSize > 1024 {
		return msg, fmt.Errorf("unexpectedly big response size: %d", msgSize)
	}
	if cap(msg) < msgSize {
		msg = make([]byte, msgSize)
	}
	msg = msg[:msgSize]
	n, err = io.ReadFull(br, msg)
	if err != nil {
		return msg, fmt.Errorf("cannot read response body: %s", err)
	}
	if n != len(msg) {
		return msg, fmt.Errorf("unexpected number of bytes read when reading message body: %d. Expecting %d", n, len(msg))
	}
	return msg, nil
}

func TestClientNoServer(t *testing.T) {
	c := &Client{
		NewResponse: func() ResponseReader {
			return &testResponse{}
		},
		Dial: func(addr string) (net.Conn, error) {
			return nil, fmt.Errorf("no server")
		},
	}

	const iterations = 100
	deadline := time.Now().Add(50 * time.Millisecond)
	resultCh := make(chan error, iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			var req testRequest
			var resp testResponse
			req.b = []byte("foobar")
			resultCh <- c.DoDeadline(&req, &resp, deadline)
		}()
	}

	for i := 0; i < iterations; i++ {
		var err error
		select {
		case err = <-resultCh:
		case <-time.After(time.Second):
			t.Fatalf("timeout")
		}
		if err == nil {
			t.Fatalf("expecting error on iteration %d", i)
		}
		switch {
		case err == ErrTimeout:
		case strings.Contains(err.Error(), "no server"):
		default:
			t.Fatalf("unexpected error on iteration %d: %s", i, err)
		}
	}
}

func TestClientTimeout(t *testing.T) {
	dialCh := make(chan struct{})
	c := &Client{
		NewResponse: func() ResponseReader {
			return &testResponse{}
		},
		Dial: func(addr string) (net.Conn, error) {
			<-dialCh
			return nil, fmt.Errorf("no dial")
		},
	}

	const iterations = 100
	deadline := time.Now().Add(50 * time.Millisecond)
	resultCh := make(chan error, iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			var req testRequest
			var resp testResponse
			req.b = []byte("foobar")
			resultCh <- c.DoDeadline(&req, &resp, deadline)
		}()
	}

	for i := 0; i < iterations; i++ {
		var err error
		select {
		case err = <-resultCh:
		case <-time.After(time.Second):
			t.Fatalf("timeout")
		}
		if err == nil {
			t.Fatalf("expecting error on iteration %d", i)
		}
		switch {
		case err == ErrTimeout:
		default:
			t.Fatalf("unexpected error on iteration %d: %s", i, err)
		}
	}

	close(dialCh)
}

func TestClientBrokenServerCloseConn(t *testing.T) {
	testClientBrokenServer(t, func(conn net.Conn) error {
		err := conn.Close()
		if err != nil {
			err = fmt.Errorf("cannot close client connection: %s", err)
		}
		return err
	})
}

func TestClientBrokenServerGarbageResponse(t *testing.T) {
	testClientBrokenServer(t, func(conn net.Conn) error {
		_, err := conn.Write([]byte("garbage\naaaa"))
		if err != nil {
			err = fmt.Errorf("cannot send garbage to the client: %s", err)
		}
		return err
	})
}

func TestClientBrokenServerCheckRequest(t *testing.T) {
	testClientBrokenServer(t, func(conn net.Conn) error {
		var reqID [4]byte
		_, err := io.ReadFull(conn, reqID[:])
		if err != nil {
			return fmt.Errorf("cannot read reqID from the client: %s", err)
		}

		var req testRequest
		br := bufio.NewReader(conn)
		if err = req.ReadRequest(br); err != nil {
			return fmt.Errorf("cannot read request from the client: %s", err)
		}
		if string(req.b) != "foobar" {
			return fmt.Errorf("invalid request: %q. Expecting %q", req.b, "foobar")
		}

		if _, err = conn.Write(reqID[:]); err != nil {
			return fmt.Errorf("cannot send reqID to the client: %s", err)
		}
		if _, err = conn.Write([]byte("invalid\nhttp\nresponse")); err != nil {
			return fmt.Errorf("cannot send invalid http response to the client: %s", err)
		}
		if err = conn.Close(); err != nil {
			return fmt.Errorf("cannot close client connection: %s", err)
		}
		return nil
	})
}

func testClientBrokenServer(t *testing.T, serverConnFunc func(net.Conn) error) {
	ln := fasthttputil.NewInmemoryListener()
	c := &Client{
		ProtocolVersion: 123,
		SniffHeader:     "xxxxsss",
		NewResponse: func() ResponseReader {
			return &testResponse{}
		},
		Dial: func(addr string) (net.Conn, error) {
			return ln.Dial()
		},
		CompressType: CompressNone,
	}

	serverStopCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverStopCh <- err
			return
		}
		cfg := &handshakeConfig{
			conn:              conn,
			writeCompressType: c.CompressType,
			protocolVersion:   c.ProtocolVersion,
			sniffHeader:       []byte(c.SniffHeader),
		}
		readCompressType, realConn, err := handshakeServer(cfg)
		if err != nil {
			serverStopCh <- err
			return
		}
		if readCompressType != c.CompressType {
			serverStopCh <- fmt.Errorf("unexpected read CompressType: %v. Expecting %v", readCompressType, c.CompressType)
			return
		}
		serverStopCh <- serverConnFunc(realConn)
	}()

	var req testRequest
	var resp testResponse
	req.b = []byte("foobar")
	err := c.DoDeadline(&req, &resp, time.Now().Add(50*time.Millisecond))
	if err == nil {
		t.Fatalf("expecting error")
	}

	// wait for the server
	ln.Close()
	select {
	case err := <-serverStopCh:
		if err != nil {
			t.Fatalf("error on the server: %s", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout")
	}
}
