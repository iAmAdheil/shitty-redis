// implements a quicklist
package list

import "github.com/codecrafters-io/redis-starter-go/app/structures/listpack"

type Quicklist struct {
	Count    int
	NumNodes int
	Head     *Node
	Tail     *Node
}

type Node struct {
	Prev     *Node
	Next     *Node
	Listpack *listpack.Listpack
}

func NewQuicklist() *Quicklist {
	return &Quicklist{
		Count:    0,
		NumNodes: 0,
		Head:     nil,
		Tail:     nil,
	}
}
