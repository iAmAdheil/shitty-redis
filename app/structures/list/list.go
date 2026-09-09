// implements a quicklist
package list

import "github.com/codecrafters-io/redis-starter-go/app/structures/listpack"

type List struct {
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

func NewQuicklist() *List {
	return &List{
		Count:    0,
		NumNodes: 0,
		Head:     nil,
		Tail:     nil,
	}
}

func NewNode() *Node {
	return &Node{
		Prev:     nil,
		Next:     nil,
		Listpack: listpack.NewListpack(),
	}
}

func (q *List) RPUSH(items []string) {
	var node *Node
	// pick target node
	if q.Head == nil && q.Tail == nil {
		node = NewNode()
		q.Head = node
		q.Tail = node
	} else {
		node = q.Tail
	}

	for _, v := range items {
		if err := node.Listpack.PushR(v); err != nil {
			// create a new node and make it the tail node
			// next entries will go into this node
			node = NewNode()
			q.Tail.Next = node
			node.Prev = q.Tail
			q.Tail = node

			// unexpected error
			// inserting into an empty listpack
			if err := node.Listpack.PushR(v); err != nil {
				panic("Unexpected error: inserting element into an empty Listpack")
			}
		}
	}
}
