package stream

import (
	"encoding/binary"
	"fmt"

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

func (st *Stream) XRANGE(l, h string) ([]string, error) {
	res := []string{} // hold all entries
	entries := []*streamlistpack.Entry{}

	l, h = formatXRANGEIds(l, h)
	// ids for stream listpack entry comparison
	msL, seqL, err := splitId(l)
	if err != nil {
		return nil, err
	}
	msH, seqH, err := splitId(h)
	if err != nil {
		return nil, err
	}

	low, high := []byte{}, []byte{}
	// ids for radix node search
	low = binary.BigEndian.AppendUint64(low, msL)
	low = binary.BigEndian.AppendUint64(low, seqL)
	high = binary.BigEndian.AppendUint64(high, msH)
	high = binary.BigEndian.AppendUint64(high, seqH)

	cur := st.Rax.FindPredRax(low) // starting rax node
	for {
		// if no pred exists || raxnode head greater than high
		if cur == nil || msH < cur.Ms || (msH == cur.Ms && seqH < cur.Seq) {
			break
		}

		e := cur.ReadAllInRange(msL, seqL, msH, seqH)
		var msEnd uint64
		var seqEnd uint64
		if len(e) > 0 {
			entries = append(entries, e...)

			// get ms and seq for last entry in
			msEnd = e[len(e)-1].Ms
			seqEnd = e[len(e)-1].Seq
		} else {
			msEnd = cur.Ms
			seqEnd = cur.Seq
		}

		next := []byte{}
		next = binary.BigEndian.AppendUint64(next, msEnd)
		next = binary.BigEndian.AppendUint64(next, seqEnd+1)

		cur = st.Rax.FindSucRax(next)
	}

	for _, entry := range entries {
		id := fmt.Sprintf("%d-%d", entry.Ms, entry.Seq)

		var s string = "*2\r\n"
		s += fmt.Sprintf("$%d\r\n%s\r\n", len(id), id)

		s += fmt.Sprintf("*%d\r\n", len(entry.Data))

		for i := 0; i < len(entry.Data); i += 2 {
			s += fmt.Sprintf("$%d\r\n%s\r\n", len(entry.Data[i]), entry.Data[i])
			s += fmt.Sprintf("$%d\r\n%s\r\n", len(entry.Data[i+1]), entry.Data[i+1])
		}

		res = append(res, s)
	}

	return res, nil
}
