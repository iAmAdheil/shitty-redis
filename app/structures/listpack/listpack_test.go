package listpack

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestNewListpack_InitialState(t *testing.T) {
	lp := NewListpack()

	if got := lp.GetSize(); got != 7 {
		t.Errorf("GetSize() = %d, want 7 (empty header + terminator byte)", got)
	}
	if got := lp.GetCount(); got != 0 {
		t.Errorf("GetCount() = %d, want 0", got)
	}
	if got := lp.Read(0, 10); len(got) != 0 {
		t.Errorf("Read(0,10) on an empty listpack = %v, want empty", got)
	}
}

func TestPushR_SingleString(t *testing.T) {
	lp := NewListpack()
	if err := lp.PushR("hello"); err != nil {
		t.Fatalf("PushR returned an error: %v", err)
	}

	got := lp.Read(0, 1)
	want := []string{"hello"}
	if !equal(got, want) {
		t.Errorf("Read(0,1) = %v, want %v", got, want)
	}
}

func TestPushR_MultipleStrings_PreserveOrder(t *testing.T) {
	lp := NewListpack()
	items := []string{"a", "b", "c"}
	for _, v := range items {
		if err := lp.PushR(v); err != nil {
			t.Fatalf("PushR(%q) returned an error: %v", v, err)
		}
	}

	got := lp.Read(0, len(items))
	if !equal(got, items) {
		t.Errorf("Read(0,%d) = %v, want %v", len(items), got, items)
	}
}

func TestPushL_SingleString(t *testing.T) {
	lp := NewListpack()
	if err := lp.PushL("hello"); err != nil {
		t.Fatalf("PushL returned an error: %v", err)
	}

	got := lp.Read(0, 1)
	want := []string{"hello"}
	if !equal(got, want) {
		t.Errorf("Read(0,1) = %v, want %v", got, want)
	}
}

func TestPushL_MultipleStrings_PrependOrder(t *testing.T) {
	lp := NewListpack()

	// Each PushL call must insert at the front. The last item pushed
	// must come back first when the listpack is read.
	for _, v := range []string{"a", "b", "c"} {
		if err := lp.PushL(v); err != nil {
			t.Fatalf("PushL(%q) returned an error: %v", v, err)
		}
	}

	got := lp.Read(0, 3)
	want := []string{"c", "b", "a"}
	if !equal(got, want) {
		t.Errorf("Read(0,3) = %v, want %v", got, want)
	}
}

func TestPushR_Integers_RoundTrip(t *testing.T) {
	// Values around each encoding boundary: 7-bit, 13-bit, 16-bit,
	// 24-bit, 32-bit, and 64-bit ranges.
	values := []int64{
		0, 1, 127, 128, -1,
		4095, 4096, -4096, -4097,
		32767, 32768, -32768, -32769,
		8388607, 8388608, -8388608, -8388609,
		2147483647, 2147483648, -2147483648, -2147483649,
		math.MaxInt64, math.MinInt64,
	}

	for _, v := range values {
		s := strconv.FormatInt(v, 10)
		t.Run(s, func(t *testing.T) {
			lp := NewListpack()
			if err := lp.PushR(s); err != nil {
				t.Fatalf("PushR(%q) returned an error: %v", s, err)
			}

			got := lp.Read(0, 1)
			want := []string{s}
			if !equal(got, want) {
				t.Errorf("Read(0,1) = %v, want %v", got, want)
			}
		})
	}
}

func TestPushR_Strings_VariousLengths(t *testing.T) {
	// Lengths around each string-length encoding boundary: 6-bit,
	// 12-bit, and 32-bit length headers.
	lengths := []int{0, 1, 63, 64, 65, 4095, 4096, 5000}

	for _, l := range lengths {
		t.Run(strconv.Itoa(l), func(t *testing.T) {
			lp := NewListpack()
			s := strings.Repeat("x", l)
			if err := lp.PushR(s); err != nil {
				t.Fatalf("PushR(len=%d) returned an error: %v", l, err)
			}

			got := lp.Read(0, 1)
			want := []string{s}
			if !equal(got, want) {
				t.Errorf("Read(0,1) length = %d, want length %d", len(got), len(want))
			}
		})
	}
}

func TestRead_WithOffset(t *testing.T) {
	lp := NewListpack()
	for _, v := range []string{"a", "b", "c", "d"} {
		if err := lp.PushR(v); err != nil {
			t.Fatalf("PushR(%q) returned an error: %v", v, err)
		}
	}

	got := lp.Read(1, 2)
	want := []string{"b", "c"}
	if !equal(got, want) {
		t.Errorf("Read(1,2) = %v, want %v", got, want)
	}
}

func TestRead_MoreThanAvailable(t *testing.T) {
	lp := NewListpack()
	items := []string{"a", "b", "c"}
	for _, v := range items {
		if err := lp.PushR(v); err != nil {
			t.Fatalf("PushR(%q) returned an error: %v", v, err)
		}
	}

	got := lp.Read(0, 100)
	if !equal(got, items) {
		t.Errorf("Read(0,100) = %v, want %v", got, items)
	}
}

func TestRead_ZeroCount(t *testing.T) {
	lp := NewListpack()
	if err := lp.PushR("a"); err != nil {
		t.Fatalf("PushR returned an error: %v", err)
	}

	got := lp.Read(0, 0)
	if len(got) != 0 {
		t.Errorf("Read(0,0) = %v, want empty", got)
	}
}

func TestPushR_FullListpack_ReturnsError(t *testing.T) {
	lp := NewListpack()
	big := strings.Repeat("x", 4000)

	pushed := 0
	var lastErr error
	for i := 0; i < 10; i++ {
		if lastErr = lp.PushR(big); lastErr != nil {
			break
		}
		pushed++
	}

	if pushed == 0 {
		t.Fatalf("expected at least one successful push before the listpack filled up")
	}
	if lastErr == nil {
		t.Errorf("PushR on a full listpack must return an error instead of growing past MAX_NODE_SIZE")
	}
}

func TestPushL_FullListpack_ReturnsError(t *testing.T) {
	lp := NewListpack()
	big := strings.Repeat("x", 4000)

	pushed := 0
	var lastErr error
	for i := 0; i < 10; i++ {
		if lastErr = lp.PushL(big); lastErr != nil {
			break
		}
		pushed++
	}

	if pushed == 0 {
		t.Fatalf("expected at least one successful push before the listpack filled up")
	}
	if lastErr == nil {
		t.Errorf("PushL on a full listpack must return an error instead of growing past MAX_NODE_SIZE")
	}
}

func TestGetSize_TracksBytesAdded(t *testing.T) {
	lp := NewListpack()
	before := lp.GetSize()

	entryBytes := getEntry("hello")
	if err := lp.PushR("hello"); err != nil {
		t.Fatalf("PushR returned an error: %v", err)
	}

	after := lp.GetSize()
	want := before + uint32(len(entryBytes))
	if after != want {
		t.Errorf("GetSize() after one push = %d, want %d (size must grow by the entry's encoded byte length)", after, want)
	}
}

func TestGetCount_TracksEntriesAdded(t *testing.T) {
	lp := NewListpack()
	items := []string{"a", "b", "c"}
	for _, v := range items {
		if err := lp.PushR(v); err != nil {
			t.Fatalf("PushR(%q) returned an error: %v", v, err)
		}
	}

	if got := lp.GetCount(); got != uint16(len(items)) {
		t.Errorf("GetCount() = %d, want %d", got, len(items))
	}
}

func TestPopL_SingleString(t *testing.T) {
	lp := NewListpack()
	if err := lp.PushR("hello"); err != nil {
		t.Fatalf("PushR returned an error: %v", err)
	}

	got, err := lp.PopL()
	if err != nil {
		t.Fatalf("PopL returned an error: %v", err)
	}
	if got != "hello" {
		t.Errorf("PopL() = %q, want %q", got, "hello")
	}
	if count := lp.GetCount(); count != 0 {
		t.Errorf("GetCount() after popping the only entry = %d, want 0", count)
	}
}

func TestPopL_MultipleStrings_FIFOOrder(t *testing.T) {
	lp := NewListpack()
	items := []string{"a", "b", "c"}
	for _, v := range items {
		if err := lp.PushR(v); err != nil {
			t.Fatalf("PushR(%q) returned an error: %v", v, err)
		}
	}

	// PopL always removes from the left. Pushed via PushR (append at
	// tail), so popping must return the items in original order.
	for _, want := range items {
		got, err := lp.PopL()
		if err != nil {
			t.Fatalf("PopL() returned an error: %v", err)
		}
		if got != want {
			t.Errorf("PopL() = %q, want %q", got, want)
		}
	}
}

func TestPopL_Integer_RoundTrip(t *testing.T) {
	lp := NewListpack()
	if err := lp.PushR("42"); err != nil {
		t.Fatalf("PushR returned an error: %v", err)
	}

	got, err := lp.PopL()
	if err != nil {
		t.Fatalf("PopL returned an error: %v", err)
	}
	if got != "42" {
		t.Errorf("PopL() = %q, want %q", got, "42")
	}
}

func TestPopL_UpdatesSizeAndCount(t *testing.T) {
	lp := NewListpack()
	if err := lp.PushR("a"); err != nil {
		t.Fatalf("PushR returned an error: %v", err)
	}
	if err := lp.PushR("b"); err != nil {
		t.Fatalf("PushR returned an error: %v", err)
	}

	sizeBefore := lp.GetSize()
	if _, err := lp.PopL(); err != nil {
		t.Fatalf("PopL returned an error: %v", err)
	}

	if count := lp.GetCount(); count != 1 {
		t.Errorf("GetCount() after one pop = %d, want 1", count)
	}
	if size := lp.GetSize(); size >= sizeBefore {
		t.Errorf("GetSize() after one pop = %d, want less than %d (size must shrink by the popped entry's byte length)", size, sizeBefore)
	}
}

func TestPopL_EmptyListpack_ReturnsError(t *testing.T) {
	lp := NewListpack()

	if _, err := lp.PopL(); err == nil {
		t.Errorf("PopL on an empty listpack must return an error")
	}
}

func TestPopL_ThenPushR_StillReadable(t *testing.T) {
	lp := NewListpack()
	for _, v := range []string{"a", "b", "c"} {
		if err := lp.PushR(v); err != nil {
			t.Fatalf("PushR(%q) returned an error: %v", v, err)
		}
	}

	if _, err := lp.PopL(); err != nil {
		t.Fatalf("PopL returned an error: %v", err)
	}
	if err := lp.PushR("d"); err != nil {
		t.Fatalf("PushR(%q) returned an error: %v", "d", err)
	}

	got := lp.Read(0, 3)
	want := []string{"b", "c", "d"}
	if !equal(got, want) {
		t.Errorf("Read(0,3) after PopL+PushR = %v, want %v", got, want)
	}
}

func equal(a, b []string) bool {
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
