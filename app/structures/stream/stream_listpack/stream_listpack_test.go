package streamlistpack

import "testing"

// safeReadAllInRange guards against a panic in the decode path so one bad
// case reports as a clear failure instead of crashing the whole test binary.
func safeReadAllInRange(t *testing.T, s *StreamListpack, msL, seqL, msH, seqH uint64) (entries []*Entry, panicked bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			t.Logf("ReadAllInRange panicked: %v", r)
		}
	}()
	entries = s.ReadAllInRange(msL, seqL, msH, seqH)
	return
}

func TestReadAllInRange_SingleEntry_ExactRangeIncludesIt(t *testing.T) {
	s := New([]string{"field", "value"}, 5, 5)

	entries, panicked := safeReadAllInRange(t, s, 5, 5, 5, 5)
	if panicked {
		t.Fatalf("ReadAllInRange(5-5, 5-5) panicked, want the single entry back")
	}
	if len(entries) != 1 {
		t.Errorf("ReadAllInRange(5-5, 5-5) returned %d entries, want 1: the range is inclusive of both ends, and the one entry equals both", len(entries))
	}
}

func TestReadAllInRange_MultipleEntries_InclusiveBounds(t *testing.T) {
	s := New([]string{"field", "value1"}, 5, 1)
	if err := s.Push([]string{"field", "value2"}, 5, 2); err != nil {
		t.Fatalf("Push #2: %v", err)
	}
	if err := s.Push([]string{"field", "value3"}, 5, 3); err != nil {
		t.Fatalf("Push #3: %v", err)
	}

	entries, panicked := safeReadAllInRange(t, s, 5, 1, 5, 3)
	if panicked {
		t.Fatalf("ReadAllInRange(5-1, 5-3) panicked, want all 3 entries")
	}
	if len(entries) != 3 {
		t.Fatalf("ReadAllInRange(5-1, 5-3) returned %d entries, want 3 (bounds are inclusive)", len(entries))
	}

	wantSeq := []uint64{1, 2, 3}
	wantVal := []string{"value1", "value2", "value3"}
	for i := range entries {
		if entries[i].Seq != wantSeq[i] {
			t.Errorf("entries[%d].Seq = %d, want %d", i, entries[i].Seq, wantSeq[i])
		}
		if len(entries[i].Data) < 2 || entries[i].Data[1] != wantVal[i] {
			t.Errorf("entries[%d].Data = %v, want value %q", i, entries[i].Data, wantVal[i])
		}
	}
}

func TestReadAllInRange_WideRange_StillDecodesAllEntries(t *testing.T) {
	// A range far wider than the real ids never touches the ms/seq boundary
	// comparison, so this isolates the Push -> ReadEntry round trip itself
	// from the range-inclusivity logic.
	s := New([]string{"field", "value1"}, 5, 1)
	s.Push([]string{"field", "value2"}, 5, 2)
	s.Push([]string{"field", "value3"}, 5, 3)

	entries, panicked := safeReadAllInRange(t, s, 0, 0, 1000, 0)
	if panicked {
		t.Fatalf("ReadAllInRange(0-0, 1000-0) panicked, want all 3 entries")
	}
	if len(entries) != 3 {
		t.Errorf("ReadAllInRange(0-0, 1000-0) returned %d entries, want 3", len(entries))
	}
}
