package streamlistpack

import (
	"errors"
	"strconv"

	"github.com/codecrafters-io/redis-starter-go/app/structures/listpack"
)

type Entry struct {
	Ms   uint64
	Seq  uint64
	Data []string
}

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
func New(data []string, ms, seq uint64) *StreamListpack {
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
	s.Push(data, ms, seq)

	return s
}

func (s *StreamListpack) genEntry(data []string, ms, seq uint64) []byte {
	lp := 0
	flag := 0
	msDif := ms - s.Ms    // always positive
	seqDif := seq - s.Seq // always positive
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
		entry = append(entry, listpack.GetEntry(strconv.Itoa(len(fields)))...)
		lp++
	}

	for i := 0; i < len(fields); i++ {
		if !isSameFields {
			entry = append(entry, listpack.GetEntry(fields[i])...)
			lp++
		}
		entry = append(entry, listpack.GetEntry(values[i])...)
		lp++
	}

	entry = append(entry, listpack.GetEntry(strconv.Itoa(lp))...)

	return entry
}

func (s *StreamListpack) Push(data []string, ms, seq uint64) error {
	entry := s.genEntry(data, ms, seq)

	rs := STREAM_NODE_MAX_BYTES - s.lp.GetSize() // remaining size
	if rs < uint32(len(entry)) || STREAM_NODE_MAX_ENTRIES == uint32(s.Count) {
		return errors.New("Stream Listpack is full")
	}

	s.Count++

	end := s.lp.Entries[len(s.lp.Entries)-1]
	s.lp.Entries = s.lp.Entries[:len(s.lp.Entries)-1]

	s.lp.Entries = append(s.lp.Entries, entry...)
	s.lp.Entries = append(s.lp.Entries, end)
	// (@iAmAdheil) ignoring internal lp count and size for now
	// Pls update if required in the future

	return nil
}

func (s *StreamListpack) ReadEntry(offset int) (*Entry, int, error) {
	var (
		entry        *Entry
		data         []string = []string{}
		isDeleted    bool     = false
		isSameFields bool     = false
	)

	start := s.lp.Read(offset, 1)
	if len(start) == 0 {
		// stream listpack end
		return nil, 0, errors.New("You have reached stream listpack end. Please find next raxnode and continue")
	}
	flag := start[0]
	offset++

	switch flag {
	case "1":
		isDeleted = true
	case "2":
		isSameFields = true
	case "3":
		isDeleted = true
		isSameFields = true
	}

	msDif, err := strconv.ParseUint(s.lp.Read(offset, 1)[0], 10, 64)
	if err != nil {
		return nil, 0, err
	}
	offset++
	seqDif, err := strconv.ParseUint(s.lp.Read(offset, 1)[0], 10, 64)
	if err != nil {
		return nil, 0, err
	}
	offset++

	ms := s.Ms + msDif
	seq := s.Seq + seqDif

	if isSameFields {
		// extract data
		values := s.lp.Read(offset, len(s.Fields))
		for i := 0; i < len(s.Fields); i++ {
			data = append(data, s.Fields[i], values[i])
		}
		offset += len(s.Fields)
	} else {
		// extract field count
		fieldCount, err := strconv.Atoi(s.lp.Read(offset, 1)[0])
		if err != nil {
			return nil, 0, err
		}
		offset++
		// extract data
		data = append(data, s.lp.Read(offset, 2*fieldCount)...)
		offset += 2 * fieldCount
	}

	// skip lp_count
	offset++

	// ignore entry if deleted -> no error throw
	if isDeleted {
		return nil, offset, nil
	}

	entry = &Entry{
		Ms:   ms,
		Seq:  seq,
		Data: data,
	}

	return entry, offset, nil
}

// all entries within the given range -> current stream listpack
func (s *StreamListpack) ReadAllInRange(msL, seqL, msH, seqH uint64) []*Entry {
	var (
		offset  = 0
		entries = []*Entry{}
	)

	for {
		entry, uOffset, err := s.ReadEntry(offset)
		if err != nil {
			return entries
		}

		// next stream entry
		offset = uOffset

		afterLow := entry.Ms > msL || (entry.Ms == msL && entry.Seq >= seqL)
		beforeHigh := entry.Ms < msH || (entry.Ms == msH && entry.Seq <= seqH)
		if afterLow && beforeHigh {
			entries = append(entries, entry)
		}
	}
}
