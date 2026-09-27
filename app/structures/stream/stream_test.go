package stream

import (
	"encoding/binary"
	"testing"
)

func encodeId(ms, seq uint64) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b[0:8], ms)
	binary.BigEndian.PutUint64(b[8:16], seq)
	return b
}

func TestXADD_FirstEntry_CreatesNode(t *testing.T) {
	st := New()

	st.XADD([]string{"field", "value"}, "1-1")

	if st.Length != 1 {
		t.Errorf("Length = %d, want 1", st.Length)
	}
	if st.EntriesAdded != 1 {
		t.Errorf("EntriesAdded = %d, want 1", st.EntriesAdded)
	}
	if st.LastId == nil || st.LastId.ms != 1 || st.LastId.seq != 1 {
		t.Fatalf("LastId = %+v, want {ms:1 seq:1}", st.LastId)
	}

	idB := encodeId(1, 1)
	lp := st.Rax.Get(idB)
	if lp == nil {
		t.Fatalf("Rax.Get(idB) = nil, want the listpack created for id 1-1")
	}
	if lp.Ms != 1 || lp.Seq != 1 {
		t.Errorf("stored listpack master id = {%d-%d}, want {1-1}", lp.Ms, lp.Seq)
	}
}

func TestXADD_SecondEntry_ReusesNode(t *testing.T) {
	st := New()

	st.XADD([]string{"field", "value"}, "1-1")
	st.XADD([]string{"field", "value2"}, "1-2")

	if st.Length != 2 {
		t.Errorf("Length = %d, want 2", st.Length)
	}
	if st.EntriesAdded != 2 {
		t.Errorf("EntriesAdded = %d, want 2", st.EntriesAdded)
	}
	if st.LastId == nil || st.LastId.ms != 1 || st.LastId.seq != 2 {
		t.Fatalf("LastId = %+v, want {ms:1 seq:2}", st.LastId)
	}

	// Both entries land in the same listpack node, keyed by the first
	// entry's id (1-1). A second rax node keyed at 1-2 must not exist.
	if lp := st.Rax.Get(encodeId(1, 2)); lp != nil {
		t.Errorf("Rax.Get(1-2) = %+v, want nil: second entry must reuse the existing node, not create its own", lp)
	}

	lp := st.Rax.Get(encodeId(1, 1))
	if lp == nil {
		t.Fatalf("Rax.Get(1-1) = nil, want the shared listpack node")
	}
	if lp.Count != 2 {
		t.Errorf("lp.Count = %d, want 2 after two XADD calls into the same node", lp.Count)
	}
}
