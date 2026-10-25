package rdb

import (
	"encoding/binary"
	"math"
	"runtime"
	"time"
)

// auxString appends an AUX field whose key and value are both length-prefixed strings.
func auxString(b []byte, key, val string) []byte {
	b = append(b, 0xFA, byte(len(key)))
	b = append(b, key...)
	b = append(b, byte(len(val)))
	return append(b, val...)
}

// auxInt8 appends an AUX field with an 8-bit integer value (0xC0 + 1 byte).
func auxInt8(b []byte, key string, val uint8) []byte {
	b = append(b, 0xFA, byte(len(key)))
	b = append(b, key...)
	return append(b, 0xC0, val)
}

// auxInt32 appends an AUX field with a 32-bit little-endian integer value (0xC2 + 4 bytes).
func auxInt32(b []byte, key string, val uint32) []byte {
	b = append(b, 0xFA, byte(len(key)))
	b = append(b, key...)
	b = append(b, 0xC2)
	return binary.LittleEndian.AppendUint32(b, val)
}

func EmptyRDB() []byte {
	var b []byte
	b = append(b, "REDIS0011"...) // magic + version

	b = auxString(b, "redis-ver", "7.2.0")              // static: version of the writer
	b = auxInt8(b, "redis-bits", 64)                    // static: 64-bit architecture
	b = auxInt32(b, "ctime", uint32(time.Now().Unix())) // dynamic: creation time, unix seconds
	b = auxInt32(b, "used-mem", usedMem())              // dynamic: memory in use when the file is built
	b = auxInt8(b, "aof-base", 0)                       // static: 0 = not the base of an AOF file

	b = append(b, 0xFF)               // EOF
	b = append(b, make([]byte, 8)...) // checksum: 8 zero bytes
	return b
}

// usedMem reports the bytes of heap in use, capped to fit the 32-bit AUX encoding.
func usedMem() uint32 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.Alloc > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(m.Alloc)
}
