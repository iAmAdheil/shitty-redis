package list

import (
	"strconv"
	"strings"
	"testing"
)

func TestNewQuicklist_InitialState(t *testing.T) {
	q := NewQuicklist()

	if q.Count != 0 {
		t.Errorf("Count = %d, want 0", q.Count)
	}
	if q.NumNodes != 0 {
		t.Errorf("NumNodes = %d, want 0", q.NumNodes)
	}
	if q.Head != nil {
		t.Errorf("Head = %v, want nil", q.Head)
	}
	if q.Tail != nil {
		t.Errorf("Tail = %v, want nil", q.Tail)
	}
}

func TestRPUSH_Basic(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c"})

	if q.Count != 3 {
		t.Errorf("Count = %d, want 3", q.Count)
	}
	if q.NumNodes != 1 {
		t.Errorf("NumNodes = %d, want 1", q.NumNodes)
	}
	if q.Head != q.Tail {
		t.Errorf("Head and Tail must be the same node when every item fits in one listpack")
	}

	got := q.LRANGE(0, -1)
	want := []string{"a", "b", "c"}
	if !equalStrings(got, want) {
		t.Errorf("LRANGE(0,-1) = %v, want %v", got, want)
	}
}

func TestLPUSH_Basic(t *testing.T) {
	q := NewQuicklist()
	q.LPUSH([]string{"a", "b", "c"})

	if q.Count != 3 {
		t.Errorf("Count = %d, want 3", q.Count)
	}

	// LPUSH inserts items one at a time at the head. The last item
	// pushed must end up at the front of the list (matches Redis LPUSH).
	got := q.LRANGE(0, -1)
	want := []string{"c", "b", "a"}
	if !equalStrings(got, want) {
		t.Errorf("LRANGE(0,-1) = %v, want %v", got, want)
	}
}

func TestRPUSH_ThenLPUSH_Combined(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"b", "c"})
	q.LPUSH([]string{"a"})

	got := q.LRANGE(0, -1)
	want := []string{"a", "b", "c"}
	if !equalStrings(got, want) {
		t.Errorf("LRANGE(0,-1) = %v, want %v", got, want)
	}
}

func TestLRANGE_NegativeIndices(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c", "d", "e"})

	got := q.LRANGE(-3, -1)
	want := []string{"c", "d", "e"}
	if !equalStrings(got, want) {
		t.Errorf("LRANGE(-3,-1) = %v, want %v", got, want)
	}
}

func TestLRANGE_NegativeIndexClampsToStart(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c"})

	got := q.LRANGE(-100, -1)
	want := []string{"a", "b", "c"}
	if !equalStrings(got, want) {
		t.Errorf("LRANGE(-100,-1) = %v, want %v", got, want)
	}
}

func TestLRANGE_StartBeyondEnd_ReturnsEmpty(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c"})

	got := q.LRANGE(10, 20)
	if len(got) != 0 {
		t.Errorf("LRANGE(10,20) = %v, want empty", got)
	}
}

func TestLRANGE_StartAfterEnd_ReturnsEmpty(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c"})

	got := q.LRANGE(2, 1)
	if len(got) != 0 {
		t.Errorf("LRANGE(2,1) = %v, want empty", got)
	}
}

func TestLRANGE_EndBeyondSize_Clamps(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c"})

	got := q.LRANGE(0, 100)
	want := []string{"a", "b", "c"}
	if !equalStrings(got, want) {
		t.Errorf("LRANGE(0,100) = %v, want %v", got, want)
	}
}

func TestLRANGE_EmptyList(t *testing.T) {
	q := NewQuicklist()

	got := q.LRANGE(0, -1)
	if len(got) != 0 {
		t.Errorf("LRANGE(0,-1) on an empty list = %v, want empty", got)
	}
}

func TestLRANGE_SingleElement(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"only"})

	got := q.LRANGE(0, -1)
	want := []string{"only"}
	if !equalStrings(got, want) {
		t.Errorf("LRANGE(0,-1) = %v, want %v", got, want)
	}

	got = q.LRANGE(1, 5)
	if len(got) != 0 {
		t.Errorf("LRANGE(1,5) = %v, want empty", got)
	}
}

func TestRPUSH_NodeOverflow_CreatesNewNode(t *testing.T) {
	q := NewQuicklist()

	// Push enough data that one listpack node cannot hold it all, so
	// RPUSH must start a new tail node partway through.
	items := make([]string, 100)
	for i := range items {
		items[i] = strconv.Itoa(i) + strings.Repeat("x", 100)
	}
	q.RPUSH(items)

	if q.Count != len(items) {
		t.Errorf("Count = %d, want %d", q.Count, len(items))
	}
	if q.NumNodes < 2 {
		t.Errorf("NumNodes = %d, want at least 2 once the data exceeds one node's capacity", q.NumNodes)
	}
	if q.Tail.Prev == nil || q.Tail.Prev.Next != q.Tail {
		t.Errorf("Tail node is not linked back to the previous node correctly")
	}

	got := q.LRANGE(0, -1)
	if !equalStrings(got, items) {
		t.Errorf("LRANGE(0,-1) did not return every item in insertion order across nodes")
	}
}

func TestLPUSH_NodeOverflow_CreatesNewNode(t *testing.T) {
	q := NewQuicklist()

	items := make([]string, 100)
	for i := range items {
		items[i] = strconv.Itoa(i) + strings.Repeat("x", 100)
	}
	q.LPUSH(items)

	if q.Count != len(items) {
		t.Errorf("Count = %d, want %d", q.Count, len(items))
	}
	if q.NumNodes < 2 {
		t.Errorf("NumNodes = %d, want at least 2 once the data exceeds one node's capacity", q.NumNodes)
	}
	if q.Head.Next == nil || q.Head.Next.Prev != q.Head {
		t.Errorf("Head node is not linked to the next node correctly")
	}

	// LPUSH inserts one at a time at the head, so the final order is
	// the reverse of the input slice.
	want := make([]string, len(items))
	for i, v := range items {
		want[len(items)-1-i] = v
	}

	got := q.LRANGE(0, -1)
	if !equalStrings(got, want) {
		t.Errorf("LRANGE(0,-1) did not return items in the expected reversed order across nodes")
	}
}

func TestLRANGE_AcrossNodeBoundary(t *testing.T) {
	q := NewQuicklist()

	items := make([]string, 100)
	for i := range items {
		items[i] = strconv.Itoa(i) + strings.Repeat("x", 100)
	}
	q.RPUSH(items)

	// This slice should straddle two nodes once the list has split.
	mid := len(items) / 2
	got := q.LRANGE(mid-2, mid+2)
	want := items[mid-2 : mid+3]
	if !equalStrings(got, want) {
		t.Errorf("LRANGE(%d,%d) = %v, want %v", mid-2, mid+2, got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
