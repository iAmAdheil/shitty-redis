package radix

import (
	"bytes"
	"slices"

	"github.com/codecrafters-io/redis-starter-go/app/structures/listpack"
)

// fixed len keys -> 16 bytes -> leaf nodes --> hasValue: true

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

func (r *RaxNode) GetSortedKeys() []byte {
	keys := make([]byte, 0, len(r.Children))
	for i, _ := range r.Children {
		keys = append(keys, i)
	}
	slices.Sort(keys)
	return keys
}

func (r *RaxNode) IsEmpty() bool {
	// if root has no children -> empty stream
	if len(r.Children) == 0 {
		return true
	}
	return false
}

func (r *RaxNode) GetMax() *listpack.Listpack {
	cur := r
	for {
		// leaf node
		if cur.HasValue {
			return cur.Value
		}

		keys := cur.GetSortedKeys()
		maxKey := keys[len(keys)-1]

		cur = cur.Children[maxKey]
	}
}

func (r *RaxNode) Insert(id []byte, value *listpack.Listpack) {
	prev, cur := r, r // track prev & current node
	// prev to break down node

	for {
		fb := id[0]
		cur = cur.Children[fb]

		if cur == nil {
			prev.Children[fb] = New(slices.Clone(id), value)
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
			n := New(slices.Clone(id[:m]), value)
			prev.Children[fb] = n
			nb := cur.Prefix[m]
			n.Children[nb] = cur
			cur.Prefix = cur.Prefix[m:]
			return

		case m < len(cur.Prefix) && m < len(id):
			joint := New(slices.Clone(id[:m]), nil) // point at which a node gets broken down
			n := New(slices.Clone(id[m:]), value)
			prev.Children[fb] = joint

			pnb, inb := cur.Prefix[m], id[m]
			joint.Children[pnb] = cur
			joint.Children[inb] = n
			cur.Prefix = cur.Prefix[m:]
			return

		}
	}
}

// answers -> is given id the starting entry for a listpack
func (r *RaxNode) Get(id []byte) *listpack.Listpack {
	cur := r

	for {
		if len(id) == 0 {
			if cur.HasValue {
				return cur.Value
			}
			return nil
		}

		fb := id[0]
		next := cur.Children[fb]
		// current node does not have an entry
		// that continues with next byte in id
		if next == nil {
			return nil
		}
		if !bytes.HasPrefix(id, next.Prefix) {
			return nil
		}
		cur = next
		id = id[len(cur.Prefix):]
	}
}

func (r *RaxNode) Find(id []byte) (res *listpack.Listpack) {
	fb := id[0]
	var next *RaxNode

	// go to predecessor if:
	//  - key does not exist
	// 	- prefix for key's node is greater than id
	next = r.Children[fb]
	if next != nil {
		prefLen := len(next.Prefix)
		// r == 0 -> recurse downwards
		// r == 0 && len(next.Prefix) == len(id) -> return value -> exact match
		// r == -1 -> get max
		r := bytes.Compare(next.Prefix, id[:prefLen])
		if r == 0 {
			if prefLen == len(id) {
				return next.Value
			}
			b := next.Find(slices.Clone(id[prefLen:]))
			if b != nil {
				return b
			}
		} else if r == -1 {
			return next.GetMax()
		}
	}
	keys := r.GetSortedKeys()
	p, err := Predecessor(keys, fb)
	if err != nil {
		return nil
	}
	next = r.Children[p]
	return next.GetMax()
}
