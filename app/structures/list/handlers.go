package list

func (q *List) RPUSH(items []string) {
	var node *Node
	// pick target node
	if q.NumNodes == 0 {
		node = NewNode()
		q.Head = node
		q.Tail = node
		q.NumNodes++
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
			q.NumNodes++

			// unexpected error
			// inserting into an empty listpack
			if err := node.Listpack.PushR(v); err != nil {
				panic("Unexpected error: inserting element into an empty Listpack")
			}
		}
		q.Count++
	}
}

func (q *List) LPUSH(items []string) {
	var node *Node
	// pick target node
	if q.NumNodes == 0 {
		node = NewNode()
		q.Head = node
		q.Tail = node
		q.NumNodes++
	} else {
		node = q.Head
	}

	for _, v := range items {
		if err := node.Listpack.PushL(v); err != nil {
			// create a new node and make it the tail node
			// next entries will go into this node
			node = NewNode()
			q.Head.Prev = node
			node.Next = q.Head
			q.Head = node
			q.NumNodes++

			// unexpected error
			// inserting into an empty listpack
			if err := node.Listpack.PushL(v); err != nil {
				panic("Unexpected error: inserting element into an empty Listpack")
			}
		}
		q.Count++
	}
}

// only supports forward direction reading for now
// will add backward reading in the future
func (q *List) LRANGE(l, r int) (elements []string) {
	size := q.Count
	// max index for the list
	rmax := size - 1

	if l < 0 {
		l = max(0, size-(l*-1))
	}
	if r < 0 {
		r = max(0, size-(r*-1))
	}
	// l and r are +ve after normalisation

	if l > r || l > rmax {
		return nil
	}
	// limit r to rmax if greater
	r = min(rmax, r)

	var node *Node = q.Head
	// node starting index
	nL := 0
	// int should be safe uint32 -> int(64) -> no sign shift
	// padded with 0s
	// node ending index
	nR := int(node.Listpack.GetCount()) - 1

	for node != nil {
		// ignore when no relation -> node indexes completely unrelated to range
		if !(nR < l || nL > r) {
			rL := max(nL, l)
			rR := min(nR, r)

			// within the node-
			offset := rL - nL
			m := rR - rL + 1

			ele := node.Listpack.Read(offset, m)
			elements = append(elements, ele...)
		}

		node = node.Next
		if node != nil {
			nL = nR + 1
			nR = nL + int(node.Listpack.GetCount()) - 1
		}
	}

	return elements
}

func (q *List) LLEN() int {
	return q.Count
}

func (q *List) LPOP(c int) (elements []string) {
	var node *Node = q.Head

	for c > 0 && node != nil {
		ele, err := node.Listpack.PopL()
		// fails if node empty, change head, drop node
		if err != nil {
			q.Head = node.Next
			node = q.Head
			if node != nil {
				node.Prev = nil
			}
			q.NumNodes--
		} else {
			elements = append(elements, ele)
			q.Count--
			c--
		}
	}

	if node != nil && node.Listpack.GetCount() == 0 {
		q.Head = node.Next
		node = q.Head
		if node != nil {
			node.Prev = nil
		}
		q.NumNodes--
	}

	// empty list, 0 nodes left
	if q.Head == nil {
		q.Tail = nil
	}

	return elements
}
