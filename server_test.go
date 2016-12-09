package fastrpc

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
	"math/rand"
	"net"
	"testing"
	"time"
)

func TestServerBrokenClientCloseConn(t *testing.T) {
	testServerBrokenClient(t, func(conn net.Conn) error {
		if err := conn.Close(); err != nil {
			return fmt.Errorf("cannot close server connection: %s", err)
		}
		return nil
	})
}

func TestServerBrokenClientGarbageRequest(t *testing.T) {
	testServerBrokenClient(t, func(conn net.Conn) error {
		_, err := conn.Write([]byte("garbage\nrequest"))
		if err != nil {
			return fmt.Errorf("cannot send garbage to the server: %s", err)
		}
		return nil
	})
}

func TestServerBrokenClientSendRequestAndCloseConn(t *testing.T) {
	testServerBrokenClient(t, func(conn net.Conn) error {
		var reqID [4]byte
		if _, err := conn.Write(reqID[:]); err != nil {
			return fmt.Errorf("cannot send reqID to the server: %s", err)
		}

		var req testRequest
		req.b = []byte("foobar")
		bw := bufio.NewWriter(conn)
		if err := req.WriteRequest(bw); err != nil {
			return fmt.Errorf("cannot send request to the server: %s", err)
		}

		if err := conn.Close(); err != nil {
			return fmt.Errorf("cannot close server connection: %s", err)
		}
		return nil
	})
}

type nilLogger struct{}

func (nl *nilLogger) Printf(fmt string, args ...interface{}) {}

type testHandlerCtx struct {
	req  testRequest
	resp testResponse
}

func newTestHandlerCtx() HandlerCtx {
	return &testHandlerCtx{}
}

func (ctx *testHandlerCtx) ConcurrencyLimitError(concurrency int) {
	ctx.resp.b = []byte("too many requests")
}

func (ctx *testHandlerCtx) Init(conn net.Conn, logger fasthttp.Logger) {
	ctx.req.b = ctx.req.b[:0]
	ctx.resp.b = ctx.resp.b[:0]
}

func (ctx *testHandlerCtx) ReadRequest(br *bufio.Reader) error {
	return ctx.req.ReadRequest(br)
}

func (ctx *testHandlerCtx) WriteResponse(bw *bufio.Writer) error {
	return ctx.resp.WriteResponse(bw)
}

func testServerBrokenClient(t *testing.T, clientConnFunc func(net.Conn) error) {
	s := &Server{
		SniffHeader:     "wqwer",
		ProtocolVersion: 123,
		NewHandlerCtx:   newTestHandlerCtx,
		Handler:         testEchoHandler,
		Logger:          &nilLogger{},
		CompressType:    CompressNone,
	}
	serverStop, ln := newTestServerExt(s)

	clientStopCh := make(chan error, 1)
	go func() {
		conn, err := ln.Dial()
		if err != nil {
			clientStopCh <- err
			return
		}

		cfg := &handshakeConfig{
			sniffHeader:       []byte(s.SniffHeader),
			protocolVersion:   s.ProtocolVersion,
			conn:              conn,
			writeCompressType: s.CompressType,
		}
		readCompressType, realConn, err := handshakeClient(cfg)
		if err != nil {
			clientStopCh <- err
			return
		}
		if readCompressType != s.CompressType {
			clientStopCh <- fmt.Errorf("unexpected read CompressType: %v. Expecting %v", readCompressType, s.CompressType)
			return
		}
		clientStopCh <- clientConnFunc(realConn)
	}()

	select {
	case err := <-clientStopCh:
		if err != nil {
			t.Fatalf("client error: %s", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout")
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerWithoutTLS(t *testing.T) {
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       testEchoHandler,
		Logger:        &nilLogger{},
	}
	serverStop, c := newTestServerClientExt(s)
	c.TLSConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	var req testRequest
	var resp testResponse

	for i := 0; i < 10; i++ {
		req.b = []byte("foobar")
		err := c.DoDeadline(&req, &resp, time.Now().Add(time.Millisecond))
		if err == nil {
			t.Fatalf("expecting non-nil error")
		}
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerTLSUnencryptedConn(t *testing.T) {
	tlsConfig := newTestServerTLSConfig()
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       testEchoHandler,
		TLSConfig:     tlsConfig,
	}
	serverStop, c := newTestServerClientExt(s)

	if err := testGet(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerTLSSerial(t *testing.T) {
	tlsConfig := newTestServerTLSConfig()
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       testEchoHandler,
		TLSConfig:     tlsConfig,
	}
	serverStop, c := newTestServerClientExt(s)
	c.TLSConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	if err := testGet(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerTLSConcurrent(t *testing.T) {
	tlsConfig := newTestServerTLSConfig()
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       testEchoHandler,
		TLSConfig:     tlsConfig,
	}
	serverStop, c := newTestServerClientExt(s)
	c.TLSConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	if err := testServerClientConcurrent(func() error { return testGet(c) }); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerNewCtxSerial(t *testing.T) {
	serverStop, c := newTestServerClient(testNewCtxHandler)

	if err := testNewCtx(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerNewCtxConcurrent(t *testing.T) {
	serverStop, c := newTestServerClient(testNewCtxHandler)

	if err := testServerClientConcurrent(func() error { return testNewCtx(c) }); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerTimeoutSerial(t *testing.T) {
	stopCh := make(chan struct{})
	h := func(ctx HandlerCtx) HandlerCtx {
		<-stopCh
		return ctx
	}
	serverStop, c := newTestServerClient(h)

	if err := testTimeout(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	close(stopCh)

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerTimeoutConcurrent(t *testing.T) {
	stopCh := make(chan struct{})
	h := func(ctx HandlerCtx) HandlerCtx {
		<-stopCh
		return ctx
	}
	serverStop, c := newTestServerClient(h)

	if err := testServerClientConcurrent(func() error { return testTimeout(c) }); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	close(stopCh)

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerBatchDelayRequestSerial(t *testing.T) {
	serverStop, c := newTestServerClient(testEchoHandler)
	c.MaxBatchDelay = 10 * time.Millisecond

	if err := testGetBatchDelay(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerBatchDelayRequestConcurrent(t *testing.T) {
	serverStop, c := newTestServerClient(testEchoHandler)
	c.MaxBatchDelay = 10 * time.Millisecond

	if err := testServerClientConcurrent(func() error { return testGetBatchDelay(c) }); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerBatchDelayResponseSerial(t *testing.T) {
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       testEchoHandler,
		MaxBatchDelay: 10 * time.Millisecond,
	}
	serverStop, c := newTestServerClientExt(s)

	if err := testGetBatchDelay(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerBatchDelayResponseConcurrent(t *testing.T) {
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       testEchoHandler,
		MaxBatchDelay: 10 * time.Millisecond,
	}
	serverStop, c := newTestServerClientExt(s)

	if err := testServerClientConcurrent(func() error { return testGetBatchDelay(c) }); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerBatchDelayRequestResponseSerial(t *testing.T) {
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       testEchoHandler,
		MaxBatchDelay: 10 * time.Millisecond,
	}
	serverStop, c := newTestServerClientExt(s)
	c.MaxBatchDelay = 10 * time.Millisecond

	if err := testGetBatchDelay(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerBatchDelayRequestResponseConcurrent(t *testing.T) {
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       testEchoHandler,
		MaxBatchDelay: 10 * time.Millisecond,
	}
	serverStop, c := newTestServerClientExt(s)
	c.MaxBatchDelay = 10 * time.Millisecond

	if err := testServerClientConcurrent(func() error { return testGetBatchDelay(c) }); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerCompressNoneSerial(t *testing.T) {
	testServerCompressSerial(t, CompressNone, CompressNone)
}

func TestServerCompressNoneConcurrent(t *testing.T) {
	testServerCompressConcurrent(t, CompressNone, CompressNone)
}

func TestServerCompressFlateSerial(t *testing.T) {
	testServerCompressSerial(t, CompressFlate, CompressFlate)
}

func TestServerCompressFlateConcurrent(t *testing.T) {
	testServerCompressConcurrent(t, CompressFlate, CompressFlate)
}

func TestServerCompressSnappySerial(t *testing.T) {
	testServerCompressSerial(t, CompressSnappy, CompressSnappy)
}

func TestServerCompressSnappyConcurrent(t *testing.T) {
	testServerCompressConcurrent(t, CompressSnappy, CompressSnappy)
}

func TestServerCompressMixedSerial(t *testing.T) {
	testServerCompressSerial(t, CompressSnappy, CompressFlate)
	testServerCompressSerial(t, CompressNone, CompressFlate)
	testServerCompressSerial(t, CompressFlate, CompressSnappy)
	testServerCompressSerial(t, CompressSnappy, CompressNone)
}

func TestServerCompressMixedConcurrent(t *testing.T) {
	testServerCompressConcurrent(t, CompressSnappy, CompressFlate)
	testServerCompressConcurrent(t, CompressNone, CompressFlate)
	testServerCompressConcurrent(t, CompressFlate, CompressSnappy)
	testServerCompressConcurrent(t, CompressSnappy, CompressNone)
}

func testServerCompressSerial(t *testing.T, reqCompressType, respCompressType CompressType) {
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       testEchoHandler,
		CompressType:  respCompressType,
	}
	serverStop, c := newTestServerClientExt(s)
	c.CompressType = reqCompressType

	if err := testGet(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func testServerCompressConcurrent(t *testing.T, reqCompressType, respCompressType CompressType) {
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       testEchoHandler,
		CompressType:  respCompressType,
	}
	serverStop, c := newTestServerClientExt(s)
	c.CompressType = reqCompressType

	if err := testServerClientConcurrent(func() error { return testGet(c) }); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerConcurrencyLimit(t *testing.T) {
	const concurrency = 10
	doneCh := make(chan struct{})
	concurrencyCh := make(chan struct{}, concurrency)
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler: func(ctxv HandlerCtx) HandlerCtx {
			concurrencyCh <- struct{}{}
			<-doneCh
			ctx := ctxv.(*testHandlerCtx)
			ctx.resp.b = []byte("done")
			return ctx
		},
		Concurrency: concurrency,
	}
	serverStop, c := newTestServerClientExt(s)

	// issue concurrency requests to the server.
	resultCh := make(chan error, concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			var req testRequest
			var resp testResponse
			req.b = []byte("foobar")
			if err := c.DoDeadline(&req, &resp, time.Now().Add(time.Hour)); err != nil {
				resultCh <- err
				return
			}
			if string(resp.b) != "done" {
				resultCh <- fmt.Errorf("unexpected body: %q. Expecting %q", resp.b, "done")
				return
			}
			resultCh <- nil
		}()
	}

	// make sure the server called request handler for the issued requests
	for i := 0; i < concurrency; i++ {
		select {
		case <-concurrencyCh:
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout on iteration %d", i)
		}
	}

	// now all the requests must fail with 'concurrency limit exceeded'
	// error.
	for i := 0; i < 100; i++ {
		var req testRequest
		var resp testResponse
		req.b = []byte("aaa.bbb")
		if err := c.DoDeadline(&req, &resp, time.Now().Add(time.Second)); err != nil {
			t.Fatalf("unexpected error on iteration %d: %s", i, err)
		}
		if string(resp.b) != "too many requests" {
			t.Fatalf("unexpected response on iteration %d: %q. Expecting %q", i, resp.b, "too many requests")
		}
	}

	// unblock requests to the server.
	close(doneCh)
	for i := 0; i < concurrency; i++ {
		select {
		case err := <-resultCh:
			if err != nil {
				t.Fatalf("unexpected error on iteration %d: %s", i, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout on iteration %d", i)
		}
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerEchoSerial(t *testing.T) {
	serverStop, c := newTestServerClient(testEchoHandler)

	if err := testGet(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerEchoConcurrent(t *testing.T) {
	serverStop, c := newTestServerClient(testEchoHandler)

	if err := testServerClientConcurrent(func() error { return testGet(c) }); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerSleepSerial(t *testing.T) {
	serverStop, c := newTestServerClient(testSleepHandler)

	if err := testSleep(c); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerSleepConcurrent(t *testing.T) {
	serverStop, c := newTestServerClient(testSleepHandler)

	if err := testServerClientConcurrent(func() error { return testSleep(c) }); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerMultiClientsSerial(t *testing.T) {
	serverStop, ln := newTestServer(testSleepHandler)

	f := func() error {
		c := newTestClient(ln)
		return testSleep(c)
	}
	if err := testServerClientConcurrent(f); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func TestServerMultiClientsConcurrent(t *testing.T) {
	serverStop, ln := newTestServer(testSleepHandler)

	f := func() error {
		c := newTestClient(ln)
		return testServerClientConcurrent(func() error { return testSleep(c) })
	}
	if err := testServerClientConcurrent(f); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := serverStop(); err != nil {
		t.Fatalf("cannot shutdown server: %s", err)
	}
}

func testServerClientConcurrent(testFunc func() error) error {
	const concurrency = 10
	resultCh := make(chan error, concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			resultCh <- testFunc()
		}()
	}

	for i := 0; i < concurrency; i++ {
		select {
		case err := <-resultCh:
			if err != nil {
				return fmt.Errorf("unexpected error: %s", err)
			}
		case <-time.After(time.Second):
			return fmt.Errorf("timeout")
		}
	}
	return nil
}

func testGet(c *Client) error {
	return testGetExt(c, 100)
}

func testGetBatchDelay(c *Client) error {
	return testGetExt(c, 10)
}

func testGetExt(c *Client, iterations int) error {
	var req testRequest
	var resp testResponse
	for i := 0; i < iterations; i++ {
		s := fmt.Sprintf("foobar %d", i)
		req.b = []byte(s)
		err := c.DoDeadline(&req, &resp, time.Now().Add(time.Second))
		if err != nil {
			return fmt.Errorf("unexpected error on iteration %d: %s", i, err)
		}
		if string(resp.b) != s {
			return fmt.Errorf("unexpected body on iteration %d: %q. Expecting %q", i, resp.b, s)
		}
	}
	return nil
}

func testPost(c *Client) error {
	var req testRequest
	var resp testResponse
	for i := 0; i < 100; i++ {
		expectedBody := fmt.Sprintf("body number %d", i)
		req.b = []byte(expectedBody)
		err := c.DoDeadline(&req, &resp, time.Now().Add(time.Second))
		if err != nil {
			return fmt.Errorf("unexpected error on iteration %d: %s", i, err)
		}
		if string(resp.b) != expectedBody {
			return fmt.Errorf("unexpected body on iteration %d: %q. Expecting %q", i, resp.b, expectedBody)
		}
	}
	return nil
}

func testSleep(c *Client) error {
	var (
		req  testRequest
		resp testResponse
	)
	expectedBodyPrefix := []byte("slept for ")
	for i := 0; i < 10; i++ {
		req.b = []byte("fobar")
		err := c.DoDeadline(&req, &resp, time.Now().Add(time.Second))
		if err != nil {
			return fmt.Errorf("unexpected error on iteration %d: %s", i, err)
		}
		if !bytes.HasPrefix(resp.b, expectedBodyPrefix) {
			return fmt.Errorf("unexpected body prefix on iteration %d: %q. Expecting %q", i, resp.b, expectedBodyPrefix)
		}
	}
	return nil
}

func testTimeout(c *Client) error {
	var (
		req  testRequest
		resp testResponse
	)
	for i := 0; i < 10; i++ {
		req.b = []byte("fobar")
		err := c.DoDeadline(&req, &resp, time.Now().Add(10*time.Millisecond))
		if err == nil {
			return fmt.Errorf("expecting non-nil error on iteration %d", i)
		}
		if err != ErrTimeout {
			return fmt.Errorf("unexpected error: %s. Expecting %s", err, ErrTimeout)
		}
	}
	return nil
}

func testNewCtx(c *Client) error {
	var (
		req  testRequest
		resp testResponse
	)
	for i := 0; i < 10; i++ {
		req.b = []byte("fobar")
		err := c.DoDeadline(&req, &resp, time.Now().Add(100*time.Millisecond))
		if err != nil {
			return fmt.Errorf("unexpected error on iteration %d: %s", i, err)
		}
		if string(resp.b) != "new ctx!" {
			return fmt.Errorf("unexpected body on iteration %d: %q. Expecting %q", i, resp.b, "new ctx!")
		}
	}
	return nil
}

func newTestServerClient(handler func(HandlerCtx) HandlerCtx) (func() error, *Client) {
	serverStop, ln := newTestServer(handler)
	c := newTestClient(ln)
	return serverStop, c
}

func newTestServerClientExt(s *Server) (func() error, *Client) {
	serverStop, ln := newTestServerExt(s)
	c := newTestClient(ln)
	return serverStop, c
}

func newTestServer(handler func(HandlerCtx) HandlerCtx) (func() error, *fasthttputil.InmemoryListener) {
	s := &Server{
		NewHandlerCtx: newTestHandlerCtx,
		Handler:       handler,
	}
	return newTestServerExt(s)
}

func newTestServerExt(s *Server) (func() error, *fasthttputil.InmemoryListener) {
	ln := fasthttputil.NewInmemoryListener()
	serverResultCh := make(chan error, 1)
	go func() {
		serverResultCh <- s.Serve(ln)
	}()

	return func() error {
		ln.Close()
		select {
		case err := <-serverResultCh:
			if err != nil {
				return fmt.Errorf("unexpected error: %s", err)
			}
		case <-time.After(time.Second):
			return fmt.Errorf("timeout")
		}
		return nil
	}, ln
}

func newTestClient(ln *fasthttputil.InmemoryListener) *Client {
	return &Client{
		NewResponse: func() ResponseReader {
			return &testResponse{}
		},
		Dial: func(addr string) (net.Conn, error) {
			return ln.Dial()
		},
	}
}

func testNewCtxHandler(ctxv HandlerCtx) HandlerCtx {
	ctxvNew := newTestHandlerCtx()
	ctx := ctxvNew.(*testHandlerCtx)
	ctx.resp.b = []byte("new ctx!")
	return ctx
}

func testEchoHandler(ctxv HandlerCtx) HandlerCtx {
	ctx := ctxv.(*testHandlerCtx)
	ctx.resp.b = append(ctx.resp.b[:0], ctx.req.b...)
	return ctx
}

func testSleepHandler(ctxv HandlerCtx) HandlerCtx {
	sleepDuration := time.Duration(rand.Intn(30)) * time.Millisecond
	time.Sleep(sleepDuration)
	s := fmt.Sprintf("slept for %s", sleepDuration)
	ctx := ctxv.(*testHandlerCtx)
	ctx.resp.b = append(ctx.resp.b[:0], s...)
	return ctx
}

func newTestServerTLSConfig() *tls.Config {
	tlsCertFile := "./ssl-cert-snakeoil.pem"
	tlsKeyFile := "./ssl-cert-snakeoil.key"
	cert, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
	if err != nil {
		panic(fmt.Sprintf("cannot load TLS key pair from certFile=%q and keyFile=%q: %s", tlsCertFile, tlsKeyFile, err))
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	return tlsConfig
}
