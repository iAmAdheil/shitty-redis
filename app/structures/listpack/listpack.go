package listpack

import (
	"encoding/binary"
	"errors"
	"strconv"
)

type Listpack struct {
	// header size -> 6 bytes
	size    [4]byte // byte count
	count   [2]byte // entry count
	Entries []byte
}

// **Store follows big endian notation everywhere**

func New() *Listpack {
	s := [4]byte{}
	binary.BigEndian.PutUint32(s[:], 7)

	return &Listpack{
		size:    s,
		Entries: []byte{0xFF},
	}
}

// BigEndian -> right to left
func (lp *Listpack) GetSize() uint32 {
	return binary.BigEndian.Uint32(lp.size[:])
}

func (lp *Listpack) UpdateSize(v int) {
	// pad with 0s, still a +ve value
	s := int64(binary.BigEndian.Uint32(lp.size[:]))
	s = max(0, s+int64(v))

	binary.BigEndian.PutUint32(lp.size[:], uint32(s))
}

func (lp *Listpack) GetCount() uint16 {
	return binary.BigEndian.Uint16(lp.count[:])
}

func (lp *Listpack) UpdateCount(v int) {
	s := int32(binary.BigEndian.Uint16(lp.count[:]))
	s = max(0, s+int32(v))

	binary.BigEndian.PutUint16(lp.count[:], uint16(s))
}

// returns an error if the listpack does not have sufficient space for the entry to fit in
func (lp *Listpack) PushR(item string) error {
	size := lp.GetSize()

	if size >= MAX_NODE_SIZE { // no space left
		return errors.New("Listpack is full")
	}

	entry := GetEntry(item)
	eSize := uint32(len(entry))

	rs := MAX_NODE_SIZE - size // remaining listpack size
	// if esize > max node size -> new listpack, add entry
	// ignore rs
	// check rs when entry does not require a listpack of its own
	if eSize < MAX_NODE_SIZE && rs < eSize { // left out space not enough to fit entry
		return errors.New("Space not enough to fit entry in Listpack")
	}

	// extract the end delimiter and resize array
	end := lp.Entries[len(lp.Entries)-1]
	lp.Entries = lp.Entries[:len(lp.Entries)-1]
	// add entry bytes and end
	lp.Entries = append(lp.Entries, entry...)
	lp.Entries = append(lp.Entries, end)

	lp.UpdateSize(len(entry))
	lp.UpdateCount(1)

	return nil
}

func (lp *Listpack) PushL(item string) error {
	size := lp.GetSize()

	if size >= MAX_NODE_SIZE { // no space left
		return errors.New("Listpack is full")
	}

	entry := GetEntry(item)
	eSize := uint32(len(entry))

	rs := MAX_NODE_SIZE - size // remaining listpack size
	// if esize > max node size -> new listpack, add entry
	// ignore rs
	// check rs when entry does not require a listpack of its own
	if eSize < MAX_NODE_SIZE && rs < eSize { // left out space not enough to fit entry
		return errors.New("Space not enough to fit entry in Listpack")
	}

	lp.Entries = append(entry, lp.Entries...)

	lp.UpdateSize(len(entry))
	lp.UpdateCount(1)

	return nil
}

// returns significant bits from the tag (if any), byte count to be read after the tag and type isInt
// not including the backlen byte
// byte count for string -> size of val
// byte count for int -> val
func decodeTag(tag byte) (int, int, bool) {
	switch {
	case tag&0x80 == 0x0:
		return 7, 0, true
	case tag&0xC0 == 0x80:
		return 6, 0, false
	case tag&0xE0 == 0xC0:
		return 5, 1, true
	case tag&0xF0 == 0xE0:
		return 4, 1, false
	case tag == 0xF0:
		return 0, 4, false
	case tag == 0xF1:
		return 0, 2, true
	case tag == 0xF2:
		return 0, 3, true
	case tag == 0xF3:
		return 0, 4, true
	case tag == 0xF4:
		return 0, 8, true
	default:
		return 0, 0, false
	}
}

// max entries to be read from the listpack
// read all if m greater than elements in node
// read -> count of no. of elements read
func (lp *Listpack) Read(offset, m int) (elements []string) {
	var i = 0    // current index
	var read = 0 // count of total elements read

	for i < len(lp.Entries) && lp.Entries[i] != 0xFF && read < m {
		entry := []byte{}

		// i -> tag, j -> next byte
		tag := lp.Entries[i]
		j := i + 1
		// tagB -> bit count to be read from tag
		// readB -> byte count to be read after tag, diff usecase
		// for string and int
		tagB, readB, isInt := decodeTag(tag)

		if isInt {
			if offset > 0 {
				i += (1 + readB) + getBacklenByteCount(1+readB) // tag + readB + backlen byte count
				offset--
				continue
			}

			switch tagB {
			case 7:
				entry = append(entry, uint8(tag&0x7F))
			case 5:
				entry = append(entry, uint8(tag&0x1F))
			}

			for ; j <= i+readB; j++ {
				// 8(7), 16(13), 16, 24, 32, 64
				entry = append(entry, lp.Entries[j])
			}

			var val int64
			switch tagB {
			case 7:
				// 7 bit int, no possible -ve ints
				// convert the 7 bit int to a 64 bit uint (padding)
				// transform into int64 (no diff, leading bit always 0)
				val = int64(bytesToUint64BE(entry))
			default:
				bitCount := uint(tagB + 8*readB)
				val = signExtend(entry, bitCount)
			}

			elements = append(elements, strconv.FormatInt(val, 10))

		} else {
			switch tagB {
			case 6:
				entry = append(entry, uint8(tag&0x3F))
			case 4:
				entry = append(entry, uint8(tag&0x0F))
			}

			for ; j <= i+readB; j++ {
				entry = append(entry, lp.Entries[j])
			}

			byteCount := int(bytesToUint64BE(entry)) // count of total string (value) bytes to be read

			if offset > 0 {
				i += (1 + readB + byteCount) + getBacklenByteCount(1+readB+byteCount) // tag + readB + string bytes + backlen byte count
				offset--
				continue
			}

			val := []byte{}
			// read byteCounts starting from j
			for byteCount != 0 {
				val = append(val, lp.Entries[j])
				j++
				byteCount--
			}

			elements = append(elements, string(val))
		}
		read++
		// j's final position -> backlen byte
		// calculate & add backlen byte count
		i = j + getBacklenByteCount(j-i)
	}

	return elements
}

func (lp *Listpack) PopL() (string, error) {
	var (
		entry     []byte
		res       string
		byteCount int
	)

	if lp.GetCount() == 0 {
		return "", errors.New("Empty Listpack")
	}

	tag := lp.Entries[0]
	// tagB -> bit count to be read from tag
	// readB -> bit count to be read after tag, diff usecase
	// for string and int
	tagB, readB, isInt := decodeTag(tag)

	if isInt {
		byteCount = 1 + readB // tag + bytes
		byteCount += getBacklenByteCount(byteCount)

		switch tagB {
		case 7:
			entry = append(entry, uint8(tag&0x7F))
		case 5:
			entry = append(entry, uint8(tag&0x1F))
		}

		for i := 1; i <= readB; i++ {
			// 8(7), 16(13), 16, 24, 32, 64
			entry = append(entry, lp.Entries[i])
		}

		var val int64
		switch tagB {
		case 7:
			// 7 bit int, no possible -ve ints
			// convert the 7 bit int to a 64 bit uint (padding)
			// transform into int64 (no diff, leading bit always 0)
			val = int64(bytesToUint64BE(entry))
		default:
			bitCount := uint(tagB + 8*readB)
			val = signExtend(entry, bitCount)
		}

		res = strconv.FormatInt(val, 10)

	} else {
		switch tagB {
		case 6:
			entry = append(entry, uint8(tag&0x3F))
		case 4:
			entry = append(entry, uint8(tag&0x0F))
		}

		var fIdx = 0 // index to start reading string bytes from, start at tag bit
		for i := 1; i <= readB; i++ {
			entry = append(entry, lp.Entries[i])
			fIdx = i
		}
		fIdx++ // last len byte -> first string byte

		strBCount := int(bytesToUint64BE(entry)) // count of total string (value) bytes to be read
		byteCount = 1 + readB + strBCount        // tag + len bytes + string bytes
		byteCount += getBacklenByteCount(byteCount)

		val := []byte{}
		// read strBCount bytes starting from fIdx
		for strBCount != 0 {
			val = append(val, lp.Entries[fIdx])
			fIdx++
			strBCount--
		}

		res = string(val)
	}

	lp.Entries = lp.Entries[byteCount:]
	lp.UpdateCount(-1)
	// remove bytecount bytes from listpack
	lp.UpdateSize(byteCount * -1)

	return res, nil
}
