package tlv

import (
	"bufio"
	"bytes"
	"fmt"
	"testing"
)

func TestResponseMarshalUnmarshal(t *testing.T) {
	var buf bytes.Buffer

	resp := AcquireResponse()
	bw := bufio.NewWriter(&buf)
	for i := 0; i < 10; i++ {
		value := fmt.Sprintf("value %d", i)
		resp.SwapValue([]byte(value))
		if err := resp.WriteResponse(bw); err != nil {
			t.Fatalf("unexpected error when writing response: %s", err)
		}
	}
	if err := bw.Flush(); err != nil {
		t.Fatalf("unexpected error when flushing response: %s", err)
	}
	ReleaseResponse(resp)

	resp1 := AcquireResponse()
	br := bufio.NewReader(&buf)
	for i := 0; i < 10; i++ {
		value := fmt.Sprintf("value %d", i)
		if err := resp1.ReadResponse(br); err != nil {
			t.Fatalf("unexpected error when reading response: %s", err)
		}
		if string(resp1.Value()) != value {
			t.Fatalf("unexpected request value read: %q. Expecting %q", resp1.Value(), value)
		}
	}
	ReleaseResponse(resp1)
}
