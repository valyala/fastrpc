package tlv

import (
	"bufio"
	"fmt"
	"sync"
)

// Response is a TLV response.
type Response struct {
	value []byte

	sizeBuf [4]byte
}

// Reset resets the given response.
func (resp *Response) Reset() {
	resp.value = resp.value[:0]
}

// Write appends p to the response value.
//
// It implements io.Writer.
func (resp *Response) Write(p []byte) (int, error) {
	resp.Append(p)
	return len(p), nil
}

// Append appends p to the response value.
func (resp *Response) Append(p []byte) {
	resp.value = append(resp.value, p...)
}

// SwapValue swaps the given value with the response's value.
//
// It is forbidden accessing the swapped value after the call.
func (resp *Response) SwapValue(value []byte) []byte {
	v := resp.value
	resp.value = value
	return v
}

// Value returns response value.
//
// The returned value is valid until the next Response method call.
// or until ReleaseResponse is called.
func (resp *Response) Value() []byte {
	return resp.value
}

// WriteResponse writes the response to bw.
func (resp *Response) WriteResponse(bw *bufio.Writer) error {
	if err := writeBytes(bw, resp.value, resp.sizeBuf[:]); err != nil {
		return fmt.Errorf("cannot write response value: %s", err)
	}
	return nil
}

// ReadResponse reads the response from br.
//
// It implements fastrpc.ReadResponse.
func (resp *Response) ReadResponse(br *bufio.Reader) error {
	var err error
	resp.value, err = readBytes(br, resp.value[:0], resp.sizeBuf[:])
	if err != nil {
		return fmt.Errorf("cannot read request value: %s", err)
	}
	return nil
}

// AcquireResponse acquires new response.
func AcquireResponse() *Response {
	v := responsePool.Get()
	if v == nil {
		v = &Response{}
	}
	return v.(*Response)
}

// ReleaseResponse releases the given response.
func ReleaseResponse(resp *Response) {
	resp.Reset()
	responsePool.Put(resp)
}

var responsePool sync.Pool
