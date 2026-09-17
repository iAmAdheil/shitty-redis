// implements a quicklist
package list

import "github.com/codecrafters-io/redis-starter-go/app/structures/listpack"

type List struct {
	count    int // element count
	NumNodes int
	Head     *Node
	Tail     *Node
}

type Node struct {
	Prev     *Node
	Next     *Node
	Listpack *listpack.Listpack
}

func New() *List {
	return &List{
		count:    0,
		NumNodes: 0,
		Head:     nil,
		Tail:     nil,
	}
}

func NewNode() *Node {
	return &Node{
		Prev:     nil,
		Next:     nil,
		Listpack: listpack.New(),
	}
}
