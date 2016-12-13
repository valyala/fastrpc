package tlv

import (
	"bufio"
	"fmt"
	"io"
)

const maxBytesSize = 1024 * 1024

func writeBytes(bw *bufio.Writer, b, sizeBuf []byte) error {
	size := len(b)
	if size > maxBytesSize {
		return fmt.Errorf("too big size=%d. Must not exceed %d", size, maxBytesSize)
	}
	buf := appendUint32(sizeBuf[:0], uint32(size))
	_, err := bw.Write(buf)
	if err != nil {
		return fmt.Errorf("cannot write size: %s", err)
	}
	_, err = bw.Write(b)
	if err != nil {
		return fmt.Errorf("cannot write body with size %d: %s", size, err)
	}
	return nil
}

func readBytes(br *bufio.Reader, b, sizeBuf []byte) ([]byte, error) {
	_, err := io.ReadFull(br, sizeBuf)
	if err != nil {
		return b, fmt.Errorf("cannot read size: %s", err)
	}
	size := int(bytes2Uint32(sizeBuf))
	if size > maxBytesSize {
		return b, fmt.Errorf("too big size=%d. Must not exceed %d", size, maxBytesSize)
	}
	if cap(b) < size {
		b = make([]byte, size)
	}
	b = b[:size]
	_, err = io.ReadFull(br, b)
	if err != nil {
		return b, fmt.Errorf("cannot read body with size %d: %s", size, err)
	}
	return b, nil
}

func appendUint32(b []byte, n uint32) []byte {
	return append(b, byte(n), byte(n>>8), byte(n>>16), byte(n>>24))
}

func bytes2Uint32(b []byte) uint32 {
	return (uint32(b[3]) << 24) | (uint32(b[2]) << 16) | (uint32(b[1]) << 8) | uint32(b[0])
}
