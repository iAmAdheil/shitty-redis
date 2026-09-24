package stream

import (
	"encoding/binary"

	"github.com/codecrafters-io/redis-starter-go/app/structures/radix"
	streamlistpack "github.com/codecrafters-io/redis-starter-go/app/structures/stream/stream_listpack"
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

func New() *Stream {
	return &Stream{
		Rax:               radix.New([]byte{}, nil), // root node
		Length:            0,                        // count of existing entries
		LastId:            nil,                      // last inserted id
		MaxDeletedEntryId: nil,
		EntriesAdded:      0, // total entries added, including deleted ones (History)
	}
}

func (st *Stream) XADD(data []string, idS string) {
	var lp *streamlistpack.StreamListpack
	ms, seq, _ := splitId(idS) // validated Id, can ignore error here
	id := &Id{
		ms:  ms,
		seq: seq,
	}
	idB := []byte{}
	idB = binary.BigEndian.AppendUint64(idB, ms)
	idB = binary.BigEndian.AppendUint64(idB, seq)

	st.Length++
	st.EntriesAdded++
	st.LastId = id

	lp = st.Rax.GetMax()
	if lp != nil {
		err := lp.Push(data, ms, seq)
		if err == nil {
			return // happy path
		}
	}
	// create new listpack and push
	lp = streamlistpack.New(data, ms, seq)
	st.Rax.Insert(idB, lp)
}
