package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func RESPDecoder(in []byte) (string, []string, error) {
	// print incoming bytes -> testing
	// fmt.Printf("%q\n", in[:n])

	r := bytes.NewReader(in)
	reader := bufio.NewReader(r)

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", []string{}, fmt.Errorf("error: Invalid argument length")
	}

	line = string(bytes.TrimSpace([]byte(line)))
	if len(line) == 0 || line[0] != '*' {
		return "", []string{}, fmt.Errorf("expected array, got: %s", line)
	}

	// Parse number of elements in the array
	numElements, err := strconv.Atoi(line[1:])
	if err != nil {
		return "", []string{}, fmt.Errorf("invalid array length: %s", err.Error())
	}

	var base string
	var args []string

	for i := 0; i < numElements; i++ {
		// Read the bulk string header line (e.g., "$4")
		header, err := reader.ReadString('\n')
		if err != nil {
			return "", []string{}, fmt.Errorf("error: %s\n", err.Error())
		}
		header = string(bytes.TrimSpace([]byte(header)))

		if len(header) == 0 || header[0] != '$' {
			return "", []string{}, fmt.Errorf("expected bulk string, got: %s", header)
		}

		// Parse the length
		length, err := strconv.Atoi(header[1:])
		if err != nil {
			return "", []string{}, fmt.Errorf("invalid bulk string length: %s", err.Error())
		}

		// Handle Null bulk string ($ -1)
		// (@iAmAdheil) -> pls verify behaviour and usecase
		if length == -1 {
			args = append(args, "") // or handle as nil
			continue
		}

		// Read EXACTLY 'length' bytes for the data payload
		buf := make([]byte, length)
		_, err = io.ReadFull(reader, buf)
		if err != nil {
			return "", []string{}, fmt.Errorf("failed to read bulk string data: %s", err.Error())
		}

		// 4. Consume the trailing \r\n
		trailer := make([]byte, 2)
		_, err = io.ReadFull(reader, trailer)
		if err != nil {
			return "", []string{}, fmt.Errorf("failed to read trailing CRLF: %s", err.Error())
		}

		if i == 0 {
			base = string(buf)
		} else {
			args = append(args, string(buf))
		}
	}

	return strings.ToLower(base), args, nil
}

type EncodeType int

const (
	Simple EncodeType = iota
	NullBulk
	Bulk
	Int
	BulkList
	NullBulkList
	SimpleErr
)

var typeName = map[EncodeType]string{
	Simple:       "simple",
	Bulk:         "bulk",
	Int:          "int",
	BulkList:     "bulk_list",
	NullBulk:     "null_bulk",
	NullBulkList: "null_bulk_list",
	SimpleErr:    "simple_err",
}

func (et EncodeType) String() string {
	return typeName[et]
}

// simple string -> +{string}\r\n
// bulk string -> ${string_len}\r\n{string}\r\n
// RESP integer -> :{integer (sent as a string)}\r\n
// bulk string list -> *{res count} ... ${string_len}\r\n{string}\r\n
func RESPEncoder(res []string, t EncodeType) []byte {
	var s string

	switch t {

	case Simple:
		s = fmt.Sprintf("+%s\r\n", res[0])

	case Int:
		s = fmt.Sprintf(":%s\r\n", res[0])

	case NullBulkList:
		s = "*-1\r\n"
	case BulkList:
		s = fmt.Sprintf("*%s\r\n", strconv.Itoa(len(res)))
		for _, v := range res {
			s += fmt.Sprintf("$%s\r\n%s\r\n", strconv.Itoa(len(v)), v)
		}

	case NullBulk:
		s = "$-1\r\n"
	case Bulk:
		v := res[0]
		s = fmt.Sprintf("$%s\r\n%s\r\n", strconv.Itoa(len(v)), v)
	case SimpleErr:
		msg := res[0]
		s = fmt.Sprintf("-ERR %s\r\n", msg)
	}

	return []byte(s)
}
