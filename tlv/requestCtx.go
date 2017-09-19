package tlv

import (
	"bufio"
	"fmt"
	"github.com/valyala/fasthttp"
	"net"
	"sync"
)

// RequestCtx implements fastrpc.HandlerCtx
type RequestCtx struct {
	// ConcurrencyLimitErrorHandler is called each time concurrency limit
	// is reached on the fastrpc.Server.
	ConcurrencyLimitErrorHandler func(ctx *RequestCtx, concurrency int)

	Request  Request
	Response Response

	conn   net.Conn
	logger ctxLogger
}

// ConcurrencyLimitError implements the corresponding method
// of fastrpc.HandlerCtx.
func (ctx *RequestCtx) ConcurrencyLimitError(concurrency int) {
	if ctx.ConcurrencyLimitErrorHandler != nil {
		ctx.ConcurrencyLimitErrorHandler(ctx, concurrency)
	}
}

// Init implements the corresponding method of fastrpc.HandlerCtx.
func (ctx *RequestCtx) Init(conn net.Conn, logger fasthttp.Logger) {
	ctx.Request.Reset()
	ctx.Response.Reset()
	ctx.conn = conn

	ctx.logger.ctx = ctx
	ctx.logger.logger = logger
}

// ReadRequest implements the corresponding method of fastrpc.HandlerCtx.
func (ctx *RequestCtx) ReadRequest(br *bufio.Reader) error {
	return ctx.Request.ReadRequest(br)
}

// WriteResponse implements the corresponding method of fastrpc.HandlerCtx.
func (ctx *RequestCtx) WriteResponse(bw *bufio.Writer) error {
	return ctx.Response.WriteResponse(bw)
}

// Conn returns connection associated with the current RequestCtx.
func (ctx *RequestCtx) Conn() net.Conn {
	return ctx.conn
}

// Logger returns logger associated with the current RequestCtx.
func (ctx *RequestCtx) Logger() fasthttp.Logger {
	return &ctx.logger
}

// Write appends p to ctx.Response's value.
//
// It implements io.Writer.
func (ctx *RequestCtx) Write(p []byte) (int, error) {
	return ctx.Response.Write(p)
}

// RemoteAddr returns client address for the given request.
//
// Always returns non-nil result.
func (ctx *RequestCtx) RemoteAddr() net.Addr {
	addr := ctx.conn.RemoteAddr()
	if addr == nil {
		return zeroTCPAddr
	}
	return addr
}

// LocalAddr returns server address for the given request.
//
// Always returns non-nil result.
func (ctx *RequestCtx) LocalAddr() net.Addr {
	addr := ctx.conn.LocalAddr()
	if addr == nil {
		return zeroTCPAddr
	}
	return addr
}

// RemoteIP returns client ip for the given request.
//
// Always returns non-nil result.
func (ctx *RequestCtx) RemoteIP() net.IP {
	x, ok := ctx.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return net.IPv4zero
	}
	return x.IP
}

var zeroTCPAddr = &net.TCPAddr{
	IP: net.IPv4zero,
}

var ctxLoggerLock sync.Mutex

type ctxLogger struct {
	ctx    *RequestCtx
	logger fasthttp.Logger
}

func (cl *ctxLogger) Printf(format string, args ...interface{}) {
	ctxLoggerLock.Lock()
	msg := fmt.Sprintf(format, args...)
	ctx := cl.ctx
	cl.logger.Printf("%s<->%s - %s", ctx.LocalAddr(), ctx.RemoteAddr(), msg)
	ctxLoggerLock.Unlock()
}
