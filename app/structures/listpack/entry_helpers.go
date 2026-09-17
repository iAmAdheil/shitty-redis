package listpack

import "strconv"

func getIntListpackEncoding(val int64) (tag byte, totalBytes int) {
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

func getStrListpackEncoding(val string) (byte, int) {
	l := len(val) // byte count

	switch {
	case l >= 0 && l <= 63:
		return 0x80, 1
	case l >= 64 && l <= 4095:
		return 0xE0, 2
	default:
		return 0xF0, 9
	}
}

func handleIntEntry(val int64) (entry []byte) {
	tag, totalBytes := getIntListpackEncoding(val)
	entry = append(entry, tag)

	switch totalBytes {
	case 1:
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

		entry[0] = byte1 // update tag
		entry = append(entry, byte2)
	case 3:
		uVal := uint16(val)

		byte1 := uint8(uVal >> 8)
		byte2 := uint8(uVal & 0xFF)

		entry = append(entry, byte1, byte2)

	case 4:
		uVal := uint32(val & 0xFFFFFF) // neutralise first 8 bits

		byte1 := uint8(uVal >> 16)
		byte2 := uint8(uVal >> 8)
		byte3 := uint8(uVal & 0xFF)

		entry = append(entry, byte1, byte2, byte3)

	case 5:
		uVal := uint32(val)

		for i := 3; i >= 0; i-- {
			b := uint8(uVal >> (8 * i))
			entry = append(entry, b)
		}

	default:
		uVal := uint64(val)

		for i := 7; i >= 0; i-- {
			b := uint8(uVal >> (8 * i))
			entry = append(entry, b)
		}
	}

	return entry
}

func handleStrEntry(val string) (entry []byte) {
	// else string -> capture size via tag
	tag, totalBytes := getStrListpackEncoding(val)
	entry = append(entry, tag)

	itemB := []byte(val)

	switch totalBytes {
	case 1:
		lenB := uint8(len(itemB) & 0x3F)
		byte1 := tag | lenB

		entry[0] = byte1

	case 2:
		lenB := uint16(len(itemB) & 0x0FFF)

		byte1 := tag | uint8(lenB>>8)
		byte2 := uint8(lenB)

		entry[0] = byte1
		entry = append(entry, byte2)

	default:
		lenB := uint32(len(itemB))

		for i := 3; i >= 0; i-- {
			b := uint8(lenB >> (8 * i))
			entry = append(entry, b)
		}
	}

	// append string val
	entry = append(entry, itemB...)

	return entry
}

func getEntry(item string) []byte {
	var entry []byte
	/*
		- parse value into a number, else string
		- get base tag + bytes required
		- based on base tag, perform value storage in tag or independently in header
	*/
	if val, err := strconv.ParseInt(item, 10, 64); err == nil {
		entry = handleIntEntry(val)
	} else {
		entry = handleStrEntry(item)
	}

	// add backlen -> includes backlen byte as well
	backlen := uint8(len(entry))
	entry = append(entry, backlen)

	return entry
}
