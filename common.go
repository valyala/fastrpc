package fastrpc

import (
	"bufio"
	"bytes"
	"compress/flate"
	"crypto/tls"
	"fmt"
	"github.com/golang/snappy"
	"github.com/valyala/fasthttp/stackless"
	"io"
	"net"
	"sync"
	"time"
)

const (
	// DefaultMaxPendingRequests is the default number of pending requests
	// a single Client may queue before sending them to the server.
	//
	// This parameter may be overridden by Client.MaxPendingRequests.
	DefaultMaxPendingRequests = 1000

	// DefaultConcurrency is the default maximum number of concurrent
	// Server.Handler goroutines the server may run.
	DefaultConcurrency = 10000
)

const (
	// DefaultReadBufferSize is the default size for read buffers.
	DefaultReadBufferSize = 64 * 1024

	// DefaultWriteBufferSize is the default size for write buffers.
	DefaultWriteBufferSize = 64 * 1024
)

// CompressType is a compression type used for connections.
type CompressType byte

const (
	// CompressNone disables connection compression.
	//
	// CompressNone may be used in the following cases:
	//
	//   * If network bandwidth between client and server is unlimited.
	//   * If client and server are located on the same physical host.
	//   * If other CompressType values consume a lot of CPU resources.
	//
	CompressNone = CompressType(1)

	// CompressFlate uses compress/flate with default
	// compression level for connection compression.
	//
	// CompressFlate may be used in the following cases:
	//
	//     * If network bandwidth between client and server is limited.
	//     * If client and server are located on distinct physical hosts.
	//     * If both client and server have enough CPU resources
	//       for compression processing.
	//
	CompressFlate = CompressType(0)

	// CompressSnappy uses snappy compression.
	//
	// CompressSnappy vs CompressFlate comparison:
	//
	//     * CompressSnappy consumes less CPU resources.
	//     * CompressSnappy consumes more network bandwidth.
	//
	CompressSnappy = CompressType(2)
)

type handshakeConfig struct {
	sniffHeader       []byte
	protocolVersion   byte
	conn              net.Conn
	readBufferSize    int
	writeBufferSize   int
	writeCompressType CompressType
	tlsConfig         *tls.Config
	isServer          bool
}

func newBufioConn(cfg *handshakeConfig) (*bufio.Reader, *bufio.Writer, error) {

	readCompressType, realConn, err := handshake(cfg)
	if err != nil {
		return nil, nil, err
	}

	r := io.Reader(realConn)
	switch readCompressType {
	case CompressNone:
	case CompressFlate:
		r = flate.NewReader(r)
	case CompressSnappy:
		r = snappy.NewReader(r)
	default:
		return nil, nil, fmt.Errorf("unknown read CompressType: %v", readCompressType)
	}
	readBufferSize := cfg.readBufferSize
	if readBufferSize <= 0 {
		readBufferSize = DefaultReadBufferSize
	}
	br := bufio.NewReaderSize(r, readBufferSize)

	w := io.Writer(realConn)
	switch cfg.writeCompressType {
	case CompressNone:
	case CompressFlate:
		sw := stackless.NewWriter(w, func(w io.Writer) stackless.Writer {
			zw, err := flate.NewWriter(w, flate.DefaultCompression)
			if err != nil {
				panic(fmt.Sprintf("BUG: flate.NewWriter(%d) returned non-nil err: %s", flate.DefaultCompression, err))
			}
			return zw
		})
		w = &writeFlusher{w: sw}
	case CompressSnappy:
		// From the docs at https://godoc.org/github.com/golang/snappy#NewWriter :
		// There is no need to Flush or Close such a Writer,
		// so don't wrap it into writeFlusher.
		w = stackless.NewWriter(w, func(w io.Writer) stackless.Writer {
			return snappy.NewWriter(w)
		})
	default:
		return nil, nil, fmt.Errorf("unknown write CompressType: %v", cfg.writeCompressType)
	}
	writeBufferSize := cfg.writeBufferSize
	if writeBufferSize <= 0 {
		writeBufferSize = DefaultWriteBufferSize
	}
	bw := bufio.NewWriterSize(w, writeBufferSize)
	return br, bw, nil
}

func handshake(cfg *handshakeConfig) (readCompressType CompressType, realConn net.Conn, err error) {
	handshakeFunc := handshakeClient
	if cfg.isServer {
		handshakeFunc = handshakeServer
	}
	deadline := time.Now().Add(3 * time.Second)
	if err = cfg.conn.SetWriteDeadline(deadline); err != nil {
		return 0, nil, fmt.Errorf("cannot set write timeout: %s", err)
	}
	if err = cfg.conn.SetReadDeadline(deadline); err != nil {
		return 0, nil, fmt.Errorf("cannot set read timeout: %s", err)
	}
	readCompressType, realConn, err = handshakeFunc(cfg)
	if err != nil {
		return 0, nil, fmt.Errorf("error in handshake: %s", err)
	}
	if err = cfg.conn.SetWriteDeadline(zeroTime); err != nil {
		return 0, nil, fmt.Errorf("cannot reset write timeout: %s", err)
	}
	if err = cfg.conn.SetReadDeadline(zeroTime); err != nil {
		return 0, nil, fmt.Errorf("cannot reset read timeout: %s", err)
	}
	return readCompressType, realConn, err
}

func handshakeServer(cfg *handshakeConfig) (CompressType, net.Conn, error) {
	conn := cfg.conn
	readCompressType, isTLS, err := handshakeRead(conn, cfg.sniffHeader, cfg.protocolVersion)
	if err != nil {
		return 0, nil, err
	}
	if isTLS && cfg.tlsConfig == nil {
		handshakeWrite(conn, cfg.writeCompressType, false, cfg.sniffHeader, cfg.protocolVersion)
		return 0, nil, fmt.Errorf("Cannot serve encrypted client connection. " +
			"Set Server.TLSConfig for supporting encrypted connections")
	}
	if err := handshakeWrite(conn, cfg.writeCompressType, isTLS, cfg.sniffHeader, cfg.protocolVersion); err != nil {
		return 0, nil, err
	}
	if isTLS {
		tlsConn := tls.Server(conn, cfg.tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return 0, nil, fmt.Errorf("error in TLS handshake: %s", err)
		}
		conn = tlsConn
	}
	return readCompressType, conn, nil
}

func handshakeClient(cfg *handshakeConfig) (CompressType, net.Conn, error) {
	conn := cfg.conn
	isTLS := cfg.tlsConfig != nil
	if err := handshakeWrite(conn, cfg.writeCompressType, isTLS, cfg.sniffHeader, cfg.protocolVersion); err != nil {
		return 0, nil, err
	}
	readCompressType, isTLSCheck, err := handshakeRead(conn, cfg.sniffHeader, cfg.protocolVersion)
	if err != nil {
		return 0, nil, err
	}
	if isTLS {
		if !isTLSCheck {
			return 0, nil, fmt.Errorf("Server doesn't support encrypted connections. " +
				"Set Server.TLSConfig for enabling encrypted connections on the server")
		}
		tlsConn := tls.Client(conn, cfg.tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return 0, nil, fmt.Errorf("error in TLS handshake: %s", err)
		}
		conn = tlsConn
	}
	return readCompressType, conn, nil
}

func handshakeWrite(conn net.Conn, compressType CompressType, isTLS bool, sniffHeader []byte, protocolVersion byte) error {
	if _, err := conn.Write(sniffHeader); err != nil {
		return fmt.Errorf("cannot write sniffHeader %q: %s", sniffHeader, err)
	}

	var buf [3]byte
	buf[0] = protocolVersion
	buf[1] = byte(compressType)
	if isTLS {
		buf[2] = 1
	}
	if _, err := conn.Write(buf[:]); err != nil {
		return fmt.Errorf("cannot write connection header: %s", err)
	}
	return nil
}

func handshakeRead(conn net.Conn, sniffHeader []byte, protocolVersion byte) (CompressType, bool, error) {
	sniffBuf := make([]byte, len(sniffHeader))
	if _, err := io.ReadFull(conn, sniffBuf); err != nil {
		return 0, false, fmt.Errorf("cannot read sniffHeader: %s", err)
	}
	if !bytes.Equal(sniffBuf, sniffHeader) {
		return 0, false, fmt.Errorf("invalid sniffHeader read: %q. Expecting %q", sniffBuf, sniffHeader)
	}

	var buf [3]byte
	if _, err := io.ReadFull(conn, buf[:]); err != nil {
		return 0, false, fmt.Errorf("cannot read connection header: %s", err)
	}
	if buf[0] != protocolVersion {
		return 0, false, fmt.Errorf("server returned unknown protocol version: %d", buf[0])
	}
	compressType := CompressType(buf[1])
	isTLS := buf[2] != 0

	return compressType, isTLS, nil
}

var zeroTime time.Time

type writeFlusher struct {
	w stackless.Writer
}

func (wf *writeFlusher) Write(p []byte) (int, error) {
	n, err := wf.w.Write(p)
	if err != nil {
		return n, err
	}
	if err := wf.w.Flush(); err != nil {
		return 0, err
	}
	return n, nil
}

func getFlushTimer() *time.Timer {
	v := flushTimerPool.Get()
	if v == nil {
		return time.NewTimer(time.Hour * 24)
	}
	t := v.(*time.Timer)
	resetFlushTimer(t, time.Hour*24)
	return t
}

func putFlushTimer(t *time.Timer) {
	stopFlushTimer(t)
	flushTimerPool.Put(t)
}

func resetFlushTimer(t *time.Timer, d time.Duration) {
	stopFlushTimer(t)
	t.Reset(d)
}

func stopFlushTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

var flushTimerPool sync.Pool
