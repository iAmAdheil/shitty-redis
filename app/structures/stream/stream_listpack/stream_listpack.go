package streamlistpack

import (
	"errors"
	"strconv"

	"github.com/codecrafters-io/redis-starter-go/app/structures/listpack"
)

type StreamListpack struct {
	// master entry
	Ms      uint64
	Seq     uint64
	Count   int
	Deleted int
	Fields  []string
	// stream entries
	lp *listpack.Listpack //  inner listpack methods remain hidden
}

// new stream lp is created only when inserting a new stream entry
func NewStreamLP(data []string, ms, seq uint64) *StreamListpack {
	fields := []string{}
	for i := 0; i < len(data); i = i + 2 {
		fields = append(fields, data[i])
	}

	s := &StreamListpack{
		// master entry init
		Ms:      ms,
		Seq:     seq,
		Count:   0,
		Deleted: 0,
		Fields:  fields,

		lp: listpack.New(),
	}

	return s
}

func (s *StreamListpack) genEntry(data []string, ms, seq uint64) []byte {
	lp := 0
	flag := 0
	msDif := ms - s.Ms
	seqDif := seq - s.Seq
	fields, values := getFieldsAndValues(data)

	isSameFields := s.matchMasterFields(fields)
	if isSameFields {
		flag += 2
	}

	entry := []byte{}

	entry = append(entry, listpack.GetEntry(strconv.Itoa(flag))...)
	entry = append(entry, listpack.GetEntry(strconv.FormatUint(msDif, 10))...)
	entry = append(entry, listpack.GetEntry(strconv.FormatUint(seqDif, 10))...)
	lp += 3

	if !isSameFields {
		entry = append(entry, strconv.Itoa(len(fields))...)
		lp++
	}

	for i := 0; i < len(fields); i++ {
		if !isSameFields {
			entry = append(entry, []byte(fields[i])...)
			lp++
		}
		entry = append(entry, []byte(values[i])...)
		lp++
	}

	return entry
}

func (s *StreamListpack) Push(data []string, ms, seq uint64) error {
	entry := s.genEntry(data, ms, seq)

	rs := STREAM_NODE_MAX_BYTES - s.lp.GetSize() // remaining size
	if rs < uint32(len(entry)) || STREAM_NODE_MAX_ENTRIES == uint32(s.Count) {
		return errors.New("Stream Listpack is full")
	}

	s.lp.Entries = append(s.lp.Entries, entry...)

	return nil
}
