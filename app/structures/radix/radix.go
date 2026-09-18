package radix

import "github.com/codecrafters-io/redis-starter-go/app/structures/listpack"

type RaxNode struct {
	Prefix   []byte
	Value    *listpack.Listpack
	HasValue bool
	Children map[byte]*RaxNode
}

func New(prefix []byte, value *listpack.Listpack) *RaxNode {
	var hasVal bool
	if value != nil {
		hasVal = true
	}

	return &RaxNode{
		Prefix:   prefix,
		Value:    value,
		HasValue: hasVal,
		Children: make(map[byte]*RaxNode),
	}
}

func (r *RaxNode) Insert(id []byte, value *listpack.Listpack) {
	prev, cur := r, r // track prev & current node
	// prev to break down node

	for {
		fb := id[0]
		cur = cur.Children[fb]

		if cur == nil {
			prev.Children[fb] = New(id, value)
			return
		}

		m := LCP(id, cur.Prefix)

		switch {
		case m == len(cur.Prefix) && m == len(id):
			cur.Value = value
			cur.HasValue = true
			return

		case m == len(cur.Prefix) && m < len(id):
			id = id[m:]
			prev = cur

		case m < len(cur.Prefix) && m == len(id):
			// create new node with the matching prefix value
			n := New(id[:m], value)
			prev.Children[fb] = n
			nb := cur.Prefix[m]
			n.Children[nb] = cur
			cur.Prefix = cur.Prefix[m:]
			return

		case m < len(cur.Prefix) && m < len(id):
			joint := New(id[:m], nil)
			n := New(id[m:], value)
			prev.Children[fb] = joint

			pnb, inb := cur.Prefix[m], id[m]
			joint.Children[pnb] = cur
			joint.Children[inb] = n
			cur.Prefix = cur.Prefix[m:]
			return

		}
	}
}
