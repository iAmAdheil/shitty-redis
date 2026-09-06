package listpack

import (
	"encoding/binary"
	"strconv"
)

type Listpack struct {
	// header size -> 6 bytes
	Size    [4]byte
	Count   [2]byte
	Entries []byte
}

func NewListpack() *Listpack {
	s := [4]byte{}
	binary.BigEndian.PutUint32(s[:], 7)

	return &Listpack{
		Size:    s,
		Entries: []byte{0xFF},
	}
}

func update(b *[]byte, val int) {
	switch len(*b) {
	case 2:
		oldVal := binary.BigEndian.Uint16((*b)[:])
		newVal := oldVal + uint16(val)
		binary.BigEndian.PutUint16((*b)[:], newVal)

	case 4:
		oldVal := binary.BigEndian.Uint32((*b)[:])
		newVal := oldVal + uint32(val)
		binary.BigEndian.PutUint32((*b)[:], newVal)
	}
}

func GetListpackEncoding(val int64) (tag byte, totalBytes int) {
	switch {
	case val >= 0 && val <= 127:
		return byte(val), 1
	case val >= -4096 && val <= 4095:
		return 0xC0, 2 // 5 bits will combine with value bits in implementation
	case val >= -32768 && val <= 32767:
		return 0xF1, 3
	case val >= -8388608 && val <= 8388607:
		return 0xF2, 4
	case val >= -2147483648 && val <= 2147483647:
		return 0xF3, 5
	default:
		return 0xF4, 9
	}
}

func (l *Listpack) PushRight(item string) {
	entry := []byte{}
	/*
		- parse value into a number
		- get base tag + bytes required
		- based on base tag, perform value storage in tag or independently in header
	*/
	if val, err := strconv.ParseInt(item, 10, 64); err == nil {
		tag, totalBytes := GetListpackEncoding(val)
		switch totalBytes {
		case 1:
			entry = append(entry, tag)
		case 2:
			// Mask to 13 bits (0x1FFF is 13 ones in binary)
			// This automatically converts negative numbers into their
			// 13-bit Two's Complement unsigned representation.
			// Strips the 48+3 1's at the start, final 13 bits in a 16 bit container
			uVal := uint16(val & 0x1FFF)

			// Split into high 5 bits (8 bits with first 3 bits 0) and low 8 bits
			high5 := uint8(uVal >> 8) // shifts the higher 8 bits into lower bit's positions -> extract out the first 8 bits
			low8 := uint8(uVal & 0xFF)

			// Combine high 5 bits with the 13-bit tag pattern (0xC0 / 110xxxxx)
			// stored within the tag
			// first 3 bits being 0 have no effect whatsoever
			byte1 := tag | high5 // tag
			byte2 := low8

			entry = append(entry, byte1, byte2)
		case 3:
			uVal := uint16(val)

			byte1 := uint8(uVal >> 8)
			byte2 := uint8(uVal & 0xFF)

			entry = append(entry, tag, byte1, byte2)

		case 4:
			uVal := uint32(val & 0xFFFFFF) // neutralise first 8 bits

			byte1 := uint8(uVal >> 16)
			byte2 := uint8(uVal >> 8)
			byte3 := uint8(uVal & 0xFF)

			entry = append(entry, tag, byte1, byte2, byte3)

		case 5:
			uVal := uint32(val)
			entry = append(entry, tag)

			for i := 3; i >= 0; i-- {
				b := uint8(uVal >> (8 * i))
				entry = append(entry, b)
			}

		default:
			uVal := uint64(val)
			entry = append(entry, tag)

			for i := 7; i >= 0; i-- {
				b := uint8(uVal >> (8 * i))
				entry = append(entry, b)
			}
		}
	} else {
		// else string

	}

}
