package resp

import (
	"bufio"
	"bytes"
	"fmt"
	"testing"
)

func TestRESPDecoder(t *testing.T) {
	buf := []byte("*2\r\n$4\r\nPING\r\n$3\r\nkey\r\n")
	b := bytes.NewReader(buf)
	r := bufio.NewReader(b)
	base, args, _, err := RESPDecoder(r)
	if err != nil {
		t.Logf("error: %s\n", err.Error())
	}

	fmt.Printf("base: %s, args: %v\n", base, args)
}
