package stream

import (
	"github.com/codecrafters-io/redis-starter-go/app/structures/radix"
)

type Id struct {
	ms  uint64
	seq uint64
}

type Stream struct {
	Rax               *radix.RaxNode
	Length            int
	LastId            *Id
	MaxDeletedEntryId *Id
	EntriesAdded      int
	// cgroups
}

func NewStream() *Stream {
	return &Stream{
		Rax:               radix.New([]byte{}, nil), // root node
		Length:            0,                        // count of existing entries
		LastId:            nil,                      // last inserted id
		MaxDeletedEntryId: nil,
		EntriesAdded:      0, // total entries added, including deleted ones (History)
	}
}
