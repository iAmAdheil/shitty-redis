package listpack

import "encoding/binary"

func bytesToUint64BE(b []byte) uint64 {
	var buf [8]byte         // zero-initialized
	copy(buf[8-len(b):], b) // b lands in the low-order (rightmost) bytes
	return binary.BigEndian.Uint64(buf[:])
}

func signExtend(b []byte, bits uint) int64 {
	v := bytesToUint64BE(b)

	// 0 shift for 64 bits, if negative, stored that way
	shift := 64 - bits
	return int64(v<<shift) >> shift
}
