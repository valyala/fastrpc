package tlv

import (
	"bufio"
	"github.com/valyala/fasthttp"
	"net"
)

// HandlerCtx implements fastrpc.HandlerCtx
type HandlerCtx struct {
	// ConcurrencyLimitErrorHandler is called each time concurrency limit
	// is reached on the fastrpc.Server.
	ConcurrencyLimitErrorHandler func(ctx *HandlerCtx, concurrency int)

	Request  Request
	Response Response

	conn   net.Conn
	logger fasthttp.Logger
}

// ConcurrencyLimitError implements the corresponding method
// of fastrpc.HandlerCtx.
func (ctx *HandlerCtx) ConcurrencyLimitError(concurrency int) {
	if ctx.ConcurrencyLimitErrorHandler != nil {
		ctx.ConcurrencyLimitErrorHandler(ctx, concurrency)
	}
}

// Init implements the corresponding method of fastrpc.HandlerCtx.
func (ctx *HandlerCtx) Init(conn net.Conn, logger fasthttp.Logger) {
	ctx.Request.Reset()
	ctx.Response.Reset()
	ctx.conn = conn
	ctx.logger = logger
}

// ReadRequest implements the corresponding method of fastrpc.HandlerCtx.
func (ctx *HandlerCtx) ReadRequest(br *bufio.Reader) error {
	return ctx.Request.ReadRequest(br)
}

// WriteResponse implements the corresponding method of fastrpc.HandlerCtx.
func (ctx *HandlerCtx) WriteResponse(bw *bufio.Writer) error {
	return ctx.Response.WriteResponse(bw)
}

// Conn returns connection associated with the current HandlerCtx.
func (ctx *HandlerCtx) Conn() net.Conn {
	return ctx.conn
}

// Logger returns logger associated with the current HandlerCtx.
func (ctx *HandlerCtx) Logger() fasthttp.Logger {
	return ctx.logger
}
