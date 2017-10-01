package fastrpc

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/valyala/fasthttp"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// RequestWriter is an interface for writing rpc request to buffered writer.
type RequestWriter interface {
	// WriteRequest must write request to bw.
	WriteRequest(bw *bufio.Writer) error
}

// ResponseReader is an interface for reading rpc response from buffered reader.
type ResponseReader interface {
	// ReadResponse must read response from br.
	ReadResponse(br *bufio.Reader) error
}

// Client sends rpc requests to the Server over a single connection.
//
// Use multiple clients for establishing multiple connections to the server
// if a single connection processing consumes 100% of a single CPU core
// on either multi-core client or server.
type Client struct {
	// SniffHeader is the header written to each connection established
	// to the server.
	//
	// It is expected that the server replies with the same header.
	SniffHeader string

	// ProtocolVersion is the version of RequestWriter and ResponseReader.
	//
	// The ProtocolVersion must be changed each time RequestWriter
	// or ResponseReader changes the underlying format.
	ProtocolVersion byte

	// NewResponse must return new response object.
	NewResponse func() ResponseReader

	// Addr is the Server address to connect to.
	Addr string

	// CompressType is the compression type used for requests.
	//
	// CompressFlate is used by default.
	CompressType CompressType

	// Dial is a custom function used for connecting to the Server.
	//
	// fasthttp.Dial is used by default.
	Dial func(addr string) (net.Conn, error)

	// TLSConfig is TLS (aka SSL) config used for establishing encrypted
	// connection to the server.
	//
	// Encrypted connections may be used for transferring sensitive
	// information over untrusted networks.
	//
	// By default connection to the server isn't encrypted.
	TLSConfig *tls.Config

	// MaxPendingRequests is the maximum number of pending requests
	// the client may issue until the server responds to them.
	//
	// DefaultMaxPendingRequests is used by default.
	MaxPendingRequests int

	// MaxBatchDelay is the maximum duration before pending requests
	// are sent to the server.
	//
	// Requests' batching may reduce network bandwidth usage and CPU usage.
	//
	// By default requests are sent immediately to the server.
	MaxBatchDelay time.Duration

	// Maximum duration for full response reading (including body).
	//
	// This also limits idle connection lifetime duration.
	//
	// By default response read timeout is unlimited.
	ReadTimeout time.Duration

	// Maximum duration for full request writing (including body).
	//
	// By default request write timeout is unlimited.
	WriteTimeout time.Duration

	// ReadBufferSize is the size for read buffer.
	//
	// DefaultReadBufferSize is used by default.
	ReadBufferSize int

	// WriteBufferSize is the size for write buffer.
	//
	// DefaultWriteBufferSize is used by default.
	WriteBufferSize int

	// Prioritizes new requests over old requests if MaxPendingRequests pending
	// requests is reached.
	PrioritizeNewRequests bool

	once sync.Once

	lastErrLock sync.Mutex
	lastErr     error

	pendingRequests chan *clientWorkItem

	pendingResponses     map[uint32]*clientWorkItem
	pendingResponsesLock sync.Mutex

	pendingRequestsCount uint32
}

var (
	// ErrTimeout is returned from timed out calls.
	ErrTimeout = fasthttp.ErrTimeout

	// ErrPendingRequestsOverflow is returned when Client cannot send
	// more requests to the server due to Client.MaxPendingRequests limit.
	ErrPendingRequestsOverflow = errors.New("Pending requests overflow. Increase Client.MaxPendingRequests, " +
		"reduce requests rate or speed up the server")
)

// SendNowait schedules the given request for sending to the server
// set in Client.Addr.
//
// req cannot be used after SendNowait returns and until releaseReq is called.
// releaseReq is called when the req is no longer needed and may be re-used.
//
// req cannot be re-used if releaseReq is nil.
//
// Returns true if the request is successfully scheduled for sending,
// otherwise returns false.
//
// Response for the given request is ignored.
func (c *Client) SendNowait(req RequestWriter, releaseReq func(req RequestWriter)) bool {
	c.once.Do(c.init)

	// Do not track 'nowait' request as a pending request, since it
	// has no response.

	wi := acquireClientWorkItem()
	wi.req = req
	wi.releaseReq = releaseReq
	wi.deadline = coarseTimeNow().Add(10 * time.Second)
	if err := c.enqueueWorkItem(wi); err != nil {
		releaseClientWorkItem(wi)
		return false
	}
	return true
}

// DoDeadline sends the given request to the server set in Client.Addr.
//
// ErrTimeout is returned if the server didn't return response until
// the given deadline.
func (c *Client) DoDeadline(req RequestWriter, resp ResponseReader, deadline time.Time) error {
	c.once.Do(c.init)

	n := c.incPendingRequests()

	if n >= c.maxPendingRequests() {
		c.decPendingRequests()
		return c.getError(ErrPendingRequestsOverflow)
	}

	wi := acquireClientWorkItem()
	wi.req = req
	wi.resp = resp
	wi.deadline = deadline
	if err := c.enqueueWorkItem(wi); err != nil {
		c.decPendingRequests()
		releaseClientWorkItem(wi)
		return c.getError(err)
	}

	// the client guarantees that wi.done is unblocked before deadline,
	// so do not use select with time.After here.
	//
	// This saves memory and CPU resources.
	err := <-wi.done

	releaseClientWorkItem(wi)

	c.decPendingRequests()

	return err
}

func (c *Client) enqueueWorkItem(wi *clientWorkItem) error {
	select {
	case c.pendingRequests <- wi:
		return nil
	default:
		if !c.PrioritizeNewRequests {
			return ErrPendingRequestsOverflow
		}

		// slow path
		select {
		case wiOld := <-c.pendingRequests:
			c.doneError(wiOld, ErrPendingRequestsOverflow)
			select {
			case c.pendingRequests <- wi:
				return nil
			default:
				return ErrPendingRequestsOverflow
			}
		default:
			return ErrPendingRequestsOverflow
		}
	}
}

func (c *Client) maxPendingRequests() int {
	maxPendingRequests := c.MaxPendingRequests
	if maxPendingRequests <= 0 {
		maxPendingRequests = DefaultMaxPendingRequests
	}
	return maxPendingRequests
}

func (c *Client) init() {
	if c.NewResponse == nil {
		panic("BUG: Client.NewResponse cannot be nil")
	}

	n := c.maxPendingRequests()
	c.pendingRequests = make(chan *clientWorkItem, n)
	c.pendingResponses = make(map[uint32]*clientWorkItem, n)

	go func() {
		sleepDuration := 10 * time.Millisecond
		for {
			time.Sleep(sleepDuration)
			ok1 := c.unblockStaleRequests()
			ok2 := c.unblockStaleResponses()
			if ok1 || ok2 {
				sleepDuration = time.Duration(0.7 * float64(sleepDuration))
				if sleepDuration < 10*time.Millisecond {
					sleepDuration = 10 * time.Millisecond
				}
			} else {
				sleepDuration = time.Duration(1.5 * float64(sleepDuration))
				if sleepDuration > time.Second {
					sleepDuration = time.Second
				}
			}
		}
	}()

	go c.worker()
}

func (c *Client) unblockStaleRequests() bool {
	found := false
	n := len(c.pendingRequests)
	t := time.Now()
	for i := 0; i < n; i++ {
		select {
		case wi := <-c.pendingRequests:
			if t.After(wi.deadline) {
				c.doneError(wi, ErrTimeout)
				found = true
			} else {
				if err := c.enqueueWorkItem(wi); err != nil {
					c.doneError(wi, err)
				}
			}
		default:
			return found
		}
	}
	return found
}

func (c *Client) unblockStaleResponses() bool {
	found := false
	t := time.Now()
	c.pendingResponsesLock.Lock()
	for reqID, wi := range c.pendingResponses {
		if t.After(wi.deadline) {
			c.doneError(wi, ErrTimeout)
			delete(c.pendingResponses, reqID)
			found = true
		}
	}
	c.pendingResponsesLock.Unlock()
	return found
}

// PendingRequests returns the number of pending requests at the moment.
//
// This function may be used either for informational purposes
// or for load balancing purposes.
func (c *Client) PendingRequests() int {
	return int(atomic.LoadUint32(&c.pendingRequestsCount))
}

func (c *Client) incPendingRequests() int {
	return int(atomic.AddUint32(&c.pendingRequestsCount, 1))
}

func (c *Client) decPendingRequests() {
	atomic.AddUint32(&c.pendingRequestsCount, ^uint32(0))
}

func (c *Client) worker() {
	dial := c.Dial
	if dial == nil {
		dial = fasthttp.Dial
	}
	for {
		// Wait for the first request before dialing the server.
		wi := <-c.pendingRequests
		if err := c.enqueueWorkItem(wi); err != nil {
			c.doneError(wi, err)
		}

		conn, err := dial(c.Addr)
		if err != nil {
			c.setLastError(fmt.Errorf("cannot connect to %q: %s", c.Addr, err))
			time.Sleep(time.Second)
			continue
		}
		c.setLastError(err)
		laddr := conn.LocalAddr().String()
		raddr := conn.RemoteAddr().String()
		err = c.serveConn(conn)

		// close all the pending responses, since they cannot be completed
		// after the connection is closed.
		if err == nil {
			c.setLastError(fmt.Errorf("%s<->%s: connection closed by server", laddr, raddr))
		} else {
			c.setLastError(fmt.Errorf("%s<->%s: %s", laddr, raddr, err))
		}
		c.pendingResponsesLock.Lock()
		for reqID, wi := range c.pendingResponses {
			c.doneError(wi, nil)
			delete(c.pendingResponses, reqID)
		}
		c.pendingResponsesLock.Unlock()
	}
}

func (c *Client) serveConn(conn net.Conn) error {
	cfg := &handshakeConfig{
		sniffHeader:       []byte(c.SniffHeader),
		protocolVersion:   c.ProtocolVersion,
		conn:              conn,
		readBufferSize:    c.ReadBufferSize,
		writeBufferSize:   c.WriteBufferSize,
		writeCompressType: c.CompressType,
		tlsConfig:         c.TLSConfig,
		isServer:          false,
	}
	br, bw, err := newBufioConn(cfg)
	if err != nil {
		conn.Close()
		time.Sleep(time.Second)
		return err
	}

	readerDone := make(chan error, 1)
	go func() {
		readerDone <- c.connReader(br, conn)
	}()

	writerDone := make(chan error, 1)
	stopWriterCh := make(chan struct{})
	go func() {
		writerDone <- c.connWriter(bw, conn, stopWriterCh)
	}()

	select {
	case err = <-readerDone:
		close(stopWriterCh)
		conn.Close()
		<-writerDone
	case err = <-writerDone:
		conn.Close()
		<-readerDone
	}

	return err
}

func (c *Client) connWriter(bw *bufio.Writer, conn net.Conn, stopCh <-chan struct{}) error {
	var (
		wi  *clientWorkItem
		buf [4]byte
	)

	var (
		flushTimer    = getFlushTimer()
		flushCh       <-chan time.Time
		flushAlwaysCh = make(chan time.Time)
	)
	defer putFlushTimer(flushTimer)

	close(flushAlwaysCh)
	maxBatchDelay := c.MaxBatchDelay
	if maxBatchDelay < 0 {
		maxBatchDelay = 0
	}

	writeTimeout := c.WriteTimeout
	var lastWriteDeadline time.Time
	var nextReqID uint32
	for {
		select {
		case wi = <-c.pendingRequests:
		default:
			// slow path
			select {
			case wi = <-c.pendingRequests:
			case <-stopCh:
				return nil
			case <-flushCh:
				if err := bw.Flush(); err != nil {
					return fmt.Errorf("cannot flush requests data to the server: %s", err)
				}
				flushCh = nil
				continue
			}
		}

		t := coarseTimeNow()
		if t.After(wi.deadline) {
			c.doneError(wi, ErrTimeout)
			continue
		}

		reqID := uint32(0)
		if wi.resp != nil {
			nextReqID++
			if nextReqID == 0 {
				nextReqID = 1
			}
			reqID = nextReqID
		}

		if writeTimeout > 0 {
			// Optimization: update write deadline only if more than 25%
			// of the last write deadline exceeded.
			// See https://github.com/golang/go/issues/15133 for details.
			if t.Sub(lastWriteDeadline) > (writeTimeout >> 2) {
				if err := conn.SetWriteDeadline(t.Add(writeTimeout)); err != nil {
					// do not panic here, since the error may
					// indicate that the connection is already closed
					err = fmt.Errorf("cannot update write deadline: %s", err)
					c.doneError(wi, err)
					return err
				}
				lastWriteDeadline = t
			}
		}

		b := appendUint32(buf[:0], reqID)
		if _, err := bw.Write(b); err != nil {
			err = fmt.Errorf("cannot send request ID to the server: %s", err)
			c.doneError(wi, err)
			return err
		}

		if err := wi.req.WriteRequest(bw); err != nil {
			err = fmt.Errorf("cannot send request to the server: %s", err)
			c.doneError(wi, err)
			return err
		}

		if wi.resp == nil {
			// wi is no longer needed, so release it.
			releaseClientWorkItem(wi)
		} else {
			c.pendingResponsesLock.Lock()
			if _, ok := c.pendingResponses[reqID]; ok {
				c.pendingResponsesLock.Unlock()
				err := fmt.Errorf("request ID overflow. id=%d", reqID)
				c.doneError(wi, err)
				return err
			}
			c.pendingResponses[reqID] = wi
			c.pendingResponsesLock.Unlock()
		}

		// re-arm flush channel
		if flushCh == nil && len(c.pendingRequests) == 0 {
			if maxBatchDelay > 0 {
				resetFlushTimer(flushTimer, maxBatchDelay)
				flushCh = flushTimer.C
			} else {
				flushCh = flushAlwaysCh
			}
		}
	}
}

func (c *Client) connReader(br *bufio.Reader, conn net.Conn) error {
	var (
		buf  [4]byte
		resp ResponseReader
	)

	zeroResp := c.NewResponse()

	readTimeout := c.ReadTimeout
	var lastReadDeadline time.Time
	for {
		if readTimeout > 0 {
			// Optimization: update read deadline only if more than 25%
			// of the last read deadline exceeded.
			// See https://github.com/golang/go/issues/15133 for details.
			t := coarseTimeNow()
			if t.Sub(lastReadDeadline) > (readTimeout >> 2) {
				if err := conn.SetReadDeadline(t.Add(readTimeout)); err != nil {
					// do not panic here, since the error may
					// indicate that the connection is already closed
					return fmt.Errorf("cannot update read deadline: %s", err)
				}
				lastReadDeadline = t
			}
		}

		if _, err := io.ReadFull(br, buf[:]); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("cannot read response ID: %s", err)
		}

		reqID := bytes2Uint32(buf)

		c.pendingResponsesLock.Lock()
		wi := c.pendingResponses[reqID]
		delete(c.pendingResponses, reqID)
		c.pendingResponsesLock.Unlock()

		resp = nil
		if wi != nil {
			resp = wi.resp
		}
		if resp == nil {
			resp = zeroResp
		}

		if err := resp.ReadResponse(br); err != nil {
			err = fmt.Errorf("cannot read response with ID %d: %s", reqID, err)
			if wi != nil {
				c.doneError(wi, err)
			}
			return err
		}

		if wi != nil {
			if wi.resp == nil {
				panic("BUG: clientWorkItem.resp must be non-nil")
			}
			wi.done <- nil
		}
	}
}

func (c *Client) doneError(wi *clientWorkItem, err error) {
	if wi.resp != nil {
		wi.done <- c.getError(err)
	} else {
		releaseClientWorkItem(wi)
	}
}

func (c *Client) getError(err error) error {
	c.lastErrLock.Lock()
	lastErr := c.lastErr
	c.lastErrLock.Unlock()
	if lastErr != nil {
		return lastErr
	}
	return err
}

func (c *Client) setLastError(err error) {
	c.lastErrLock.Lock()
	c.lastErr = err
	c.lastErrLock.Unlock()
}

type clientWorkItem struct {
	req        RequestWriter
	resp       ResponseReader
	releaseReq func(req RequestWriter)
	deadline   time.Time
	done       chan error
}

func acquireClientWorkItem() *clientWorkItem {
	v := clientWorkItemPool.Get()
	if v == nil {
		v = &clientWorkItem{
			done: make(chan error, 1),
		}
	}
	wi := v.(*clientWorkItem)
	if len(wi.done) != 0 {
		panic("BUG: clientWorkItem.done must be empty")
	}
	return wi
}

func releaseClientWorkItem(wi *clientWorkItem) {
	if len(wi.done) != 0 {
		panic("BUG: clientWorkItem.done must be empty")
	}
	if wi.releaseReq != nil {
		if wi.resp != nil {
			panic("BUG: clientWorkItem.resp must be nil")
		}
		wi.releaseReq(wi.req)
	}
	wi.req = nil
	wi.resp = nil
	wi.releaseReq = nil
	clientWorkItemPool.Put(wi)
}

var clientWorkItemPool sync.Pool

func appendUint32(b []byte, n uint32) []byte {
	return append(b, byte(n), byte(n>>8), byte(n>>16), byte(n>>24))
}

func bytes2Uint32(b [4]byte) uint32 {
	return (uint32(b[3]) << 24) | (uint32(b[2]) << 16) | (uint32(b[1]) << 8) | uint32(b[0])
}
