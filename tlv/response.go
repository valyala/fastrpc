package tlv

import (
	"bufio"
	"fmt"
	"sync"
)

// Response is a TLV response.
type Response struct {
	// Value is response value.
	Value []byte

	sizeBuf [4]byte
}

// Reset resets the given response.
func (resp *Response) Reset() {
	resp.Value = resp.Value[:0]
}

// Write appends p to the response value.
//
// It implements io.Writer.
func (resp *Response) Write(p []byte) (int, error) {
	resp.Value = append(resp.Value, p...)
	return len(p), nil
}

// SwapValue swaps the given value with the response's value.
//
// It is forbidden accessing the swapped value after the call.
func (resp *Response) SwapValue(value []byte) []byte {
	v := resp.Value
	resp.Value = value
	return v
}

// WriteResponse writes the response to bw.
func (resp *Response) WriteResponse(bw *bufio.Writer) error {
	if err := writeBytes(bw, resp.Value, resp.sizeBuf[:]); err != nil {
		return fmt.Errorf("cannot write response value: %s", err)
	}
	return nil
}

// ReadResponse reads the response from br.
//
// It implements fastrpc.ReadResponse.
func (resp *Response) ReadResponse(br *bufio.Reader) error {
	var err error
	resp.Value, err = readBytes(br, resp.Value[:0], resp.sizeBuf[:])
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
