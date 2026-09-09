package listpack

import (
	"encoding/binary"
	"errors"
)

type Listpack struct {
	// header size -> 6 bytes
	size    [4]byte // byte count
	count   [2]byte // entry count
	Entries []byte
}

func NewListpack() *Listpack {
	s := [4]byte{}
	binary.BigEndian.PutUint32(s[:], 7)

	return &Listpack{
		size:    s,
		Entries: []byte{0xFF},
	}
}

func (lp *Listpack) GetSize() uint32 {
	return binary.LittleEndian.Uint32(lp.size[:])
}

func (lp *Listpack) GetCount() uint16 {
	return binary.LittleEndian.Uint16(lp.count[:])
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
	return nil
}
