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

func NewListpack() *Listpack {
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

func (lp *Listpack) GetCount() uint16 {
	return binary.BigEndian.Uint16(lp.count[:])
}

// returns an error if the listpack does not have sufficient space for the entry to fit in
func (lp *Listpack) PushR(item string) error {
	size := lp.GetSize()

	if size >= MAX_NODE_SIZE { // no space left
		return errors.New("Listpack is full")
	}

	entry := getEntry(item)
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

	return nil
}

func (lp *Listpack) PushL(item string) error {
	size := lp.GetSize()

	if size >= MAX_NODE_SIZE { // no space left
		return errors.New("Listpack is full")
	}

	entry := getEntry(item)
	eSize := uint32(len(entry))

	rs := MAX_NODE_SIZE - size // remaining listpack size
	// if esize > max node size -> new listpack, add entry
	// ignore rs
	// check rs when entry does not require a listpack of its own
	if eSize < MAX_NODE_SIZE && rs < eSize { // left out space not enough to fit entry
		return errors.New("Space not enough to fit entry in Listpack")
	}

	lp.Entries = append(entry, lp.Entries...)

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
func (lp *Listpack) Read(m int) (read int, elements []string) {
	var i = 0 // current index

	for i < len(lp.Entries) && lp.Entries[i] != 0xFF && read < m {
		entry := []byte{}

		tag := lp.Entries[i]
		// tagB -> bit count to be read from tag
		// readB -> bit count to be read after tag, diff usecase
		// for string and int
		tagB, readB, isInt := decodeTag(tag)

		if isInt {
			switch tagB {
			case 7:
				entry = append(entry, uint8(tag&0x7F))
			case 5:
				entry = append(entry, uint8(tag&0x1F))
			}

			for j := i + 1; j <= i+readB; j++ {
				// 8(7), 16(13), 16, 24, 32, 64
				entry = append(entry, lp.Entries[j])
			}

			bitCount := uint(tagB + 8*readB)
			val := signExtend(entry, bitCount)

			elements = append(elements, strconv.FormatInt(val, 10))

		} else {
			switch tagB {
			case 6:
				entry = append(entry, uint8(tag&0x3F))
			case 4:
				entry = append(entry, uint8(tag&0x0F))
			}

			j := i + 1
			for ; j <= i+readB; j++ {
				entry = append(entry, lp.Entries[j])
			}

			val := []byte{}
			byteCount := bytesToUint64BE(entry) // count of total string (value) bytes to be read
			// read byteCounts starting from j
			for byteCount != 0 {
				val = append(val, lp.Entries[j])
				j++
				byteCount--
			}

			elements = append(elements, string(val))
		}
	}

	return read, elements
}

// func (lp *Listpack) ListRange(l, r int32) []string {
// 	// int16 -> int32 : no information loss + leading bit is 0
// 	// prevents count from going negative during conversion if leading bit was 1
// 	size := int32(lp.GetCount())
// 	// max index for the list
// 	rmax := size - 1

// 	if l < 0 {
// 		l = max(0, size-(l*-1))
// 	}
// 	if r < 0 {
// 		r = max(0, size-(r*-1))
// 	}

// 	if l > r || l > rmax {
// 		return nil
// 	}
// }
