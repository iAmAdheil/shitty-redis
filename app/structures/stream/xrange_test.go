package stream

import (
	"strings"
	"testing"
)

// safeXRANGE guards against a panic in the range walk so one bad case
// reports as a clear failure instead of crashing the whole test binary.
func safeXRANGE(t *testing.T, st *Stream, low, high string) (res []string, err error, panicked bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			t.Logf("XRANGE(%q, %q) panicked: %v", low, high, r)
		}
	}()
	res, err = st.XRANGE(low, high)
	return
}

func TestXRANGE_SingleEntry_ExactMatch(t *testing.T) {
	st := New()
	st.XADD([]string{"field", "value"}, "5-5")

	res, err, panicked := safeXRANGE(t, st, "5-5", "5-5")
	if panicked {
		t.Fatalf("XRANGE(5-5, 5-5) panicked, want the one entry back")
	}
	if err != nil {
		t.Fatalf("XRANGE error = %v", err)
	}
	if len(res) != 1 {
		t.Errorf("XRANGE(5-5, 5-5) returned %d entries, want 1: this is the documented way to fetch a single entry by id", len(res))
	}
}

func TestXRANGE_MultipleEntriesSameListpack(t *testing.T) {
	st := New()
	st.XADD([]string{"field", "value1"}, "5-1")
	st.XADD([]string{"field", "value2"}, "5-2")
	st.XADD([]string{"field", "value3"}, "5-3")

	res, err, panicked := safeXRANGE(t, st, "5-1", "5-3")
	if panicked {
		t.Fatalf("XRANGE(5-1, 5-3) panicked, want all 3 entries")
	}
	if err != nil {
		t.Fatalf("XRANGE error = %v", err)
	}
	if len(res) != 3 {
		t.Errorf("XRANGE(5-1, 5-3) returned %d entries, want 3", len(res))
	}
}

func TestXRANGE_StartAfterLastEntry_ReturnsEmptyNotPanic(t *testing.T) {
	st := New()
	st.XADD([]string{"field", "value"}, "5-5")

	res, err, panicked := safeXRANGE(t, st, "6-0", "10-0")
	if panicked {
		t.Fatalf("XRANGE(6-0, 10-0) panicked, want an empty result: no entry falls in this range")
	}
	if err != nil {
		t.Fatalf("XRANGE error = %v", err)
	}
	if len(res) != 0 {
		t.Errorf("XRANGE(6-0, 10-0) returned %d entries, want 0", len(res))
	}
}

func TestXRANGE_MsOnlyBounds_AutoCompletesSeq(t *testing.T) {
	st := New()
	st.XADD([]string{"field", "value1"}, "5-0")
	st.XADD([]string{"field", "value2"}, "5-1")
	st.XADD([]string{"field", "value3"}, "6-0")

	// bare ms bounds should include every seq at ms=5, per Redis's own
	// "incomplete ids" rule (low auto-completes to -0, high to -max).
	res, err, panicked := safeXRANGE(t, st, "5", "5")
	if panicked {
		t.Fatalf("XRANGE(5, 5) panicked, want the 2 entries at ms=5")
	}
	if err != nil {
		t.Fatalf("XRANGE error = %v", err)
	}
	if len(res) != 2 {
		t.Errorf("XRANGE(5, 5) returned %d entries, want 2", len(res))
	}
}

func TestXRANGE_AcrossTwoListpacks(t *testing.T) {
	st := New()
	big := strings.Repeat("x", 2000)
	st.XADD([]string{"field", big}, "1-1")
	st.XADD([]string{"field", big}, "2-1") // STREAM_NODE_MAX_BYTES is 4096: forces a second listpack
	st.XADD([]string{"field", "small"}, "3-1")

	res, err, panicked := safeXRANGE(t, st, "1-1", "3-1")
	if panicked {
		t.Fatalf("XRANGE(1-1, 3-1) panicked, want all 3 entries spanning two listpacks")
	}
	if err != nil {
		t.Fatalf("XRANGE error = %v", err)
	}
	if len(res) != 3 {
		t.Errorf("XRANGE(1-1, 3-1) returned %d entries, want 3", len(res))
	}
}

func TestXRANGE_MidStreamRange_DifferentFieldsPerEntry(t *testing.T) {
	// Mirrors the CodeCrafters "Streams - Query entries from stream" case:
	// entries with different field names, range confined to a single ms.
	st := New()
	st.XADD([]string{"pear", "strawberry"}, "0-1")
	st.XADD([]string{"mango", "apple"}, "0-2")
	st.XADD([]string{"mango", "banana"}, "0-3")
	st.XADD([]string{"grape", "pear"}, "0-4")

	res, err, panicked := safeXRANGE(t, st, "0-2", "0-4")
	if panicked {
		t.Fatalf("XRANGE(0-2, 0-4) panicked")
	}
	if err != nil {
		t.Fatalf("XRANGE error = %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("XRANGE(0-2, 0-4) returned %d entries, want 3 (0-1 is below the low bound and must be excluded): %q", len(res), res)
	}

	want := []string{
		"*2\r\n$3\r\n0-2\r\n*2\r\n$5\r\nmango\r\n$5\r\napple\r\n",
		"*2\r\n$3\r\n0-3\r\n*2\r\n$5\r\nmango\r\n$6\r\nbanana\r\n",
		"*2\r\n$3\r\n0-4\r\n*2\r\n$5\r\ngrape\r\n$4\r\npear\r\n",
	}
	for i, w := range want {
		if res[i] != w {
			t.Errorf("res[%d] = %q, want %q", i, res[i], w)
		}
	}
}
