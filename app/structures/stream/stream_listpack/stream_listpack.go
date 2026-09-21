package streamlistpack

import (
	"github.com/codecrafters-io/redis-starter-go/app/structures/listpack"
)

type StreamListpack struct {
	Ms  uint64
	Seq uint64
	lp  *listpack.Listpack //  inner listpack methods remain hidden
}

// new stream lp is created only when inserting a new stream entry
func NewStreamLP(data []string, ms, seq uint64) *StreamListpack {
	s := &StreamListpack{
		Ms:  ms,
		Seq: seq,
		lp:  listpack.New(),
	}

	s.PushMaster(data)

	return s
}
