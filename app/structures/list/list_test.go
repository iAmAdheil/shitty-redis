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

func TestLLEN_EmptyList(t *testing.T) {
	q := NewQuicklist()

	if got := q.LLEN(); got != 0 {
		t.Errorf("LLEN() on an empty list = %d, want 0", got)
	}
}

func TestLLEN_AfterPushes(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c"})

	if got := q.LLEN(); got != 3 {
		t.Errorf("LLEN() = %d, want 3", got)
	}
}

func TestLLEN_TracksPops(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c"})
	q.LPOP(2)

	if got := q.LLEN(); got != 1 {
		t.Errorf("LLEN() after popping 2 of 3 elements = %d, want 1", got)
	}
}

func TestLPOP_SingleElement(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c"})

	got := q.LPOP(1)
	want := []string{"a"}
	if !equalStrings(got, want) {
		t.Errorf("LPOP(1) = %v, want %v", got, want)
	}

	remaining := q.LRANGE(0, -1)
	wantRemaining := []string{"b", "c"}
	if !equalStrings(remaining, wantRemaining) {
		t.Errorf("LRANGE(0,-1) after LPOP(1) = %v, want %v", remaining, wantRemaining)
	}
}

func TestLPOP_MultipleElements(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c", "d", "e"})

	got := q.LPOP(3)
	want := []string{"a", "b", "c"}
	if !equalStrings(got, want) {
		t.Errorf("LPOP(3) = %v, want %v", got, want)
	}
	if q.Count != 2 {
		t.Errorf("Count after LPOP(3) = %d, want 2", q.Count)
	}
}

func TestLPOP_CountExceedsLength_PopsAllAndEmptiesList(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c"})

	got := q.LPOP(10)
	want := []string{"a", "b", "c"}
	if !equalStrings(got, want) {
		t.Errorf("LPOP(10) = %v, want %v", got, want)
	}
	if q.Count != 0 {
		t.Errorf("Count after popping more than the list holds = %d, want 0", q.Count)
	}
	if q.Head != nil {
		t.Errorf("Head = %v, want nil once the list is fully popped", q.Head)
	}
	if q.Tail != nil {
		t.Errorf("Tail = %v, want nil once the list is fully popped", q.Tail)
	}
	if q.NumNodes != 0 {
		t.Errorf("NumNodes = %d, want 0 once the list is fully popped", q.NumNodes)
	}
}

func TestLPOP_EmptyList_ReturnsEmpty(t *testing.T) {
	q := NewQuicklist()

	got := q.LPOP(1)
	if len(got) != 0 {
		t.Errorf("LPOP(1) on an empty list = %v, want empty", got)
	}
}

func TestLPOP_ZeroCount_ReturnsEmpty(t *testing.T) {
	q := NewQuicklist()
	q.RPUSH([]string{"a", "b", "c"})

	got := q.LPOP(0)
	if len(got) != 0 {
		t.Errorf("LPOP(0) = %v, want empty", got)
	}
	if q.Count != 3 {
		t.Errorf("Count after LPOP(0) = %d, want unchanged at 3", q.Count)
	}
}

func TestLPOP_AcrossNodeBoundary(t *testing.T) {
	q := NewQuicklist()

	items := make([]string, 100)
	for i := range items {
		items[i] = strconv.Itoa(i) + strings.Repeat("x", 100)
	}
	q.RPUSH(items)

	if q.NumNodes < 2 {
		t.Fatalf("NumNodes = %d, want at least 2 before popping across a node boundary", q.NumNodes)
	}

	got := q.LPOP(q.Count)
	if !equalStrings(got, items) {
		t.Errorf("LPOP(all) did not return every item in insertion order across nodes")
	}
	if q.Head != nil || q.Tail != nil || q.NumNodes != 0 {
		t.Errorf("list not fully drained: Head=%v Tail=%v NumNodes=%d", q.Head, q.Tail, q.NumNodes)
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
