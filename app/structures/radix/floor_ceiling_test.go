package radix

import (
	"testing"

	streamlistpack "github.com/codecrafters-io/redis-starter-go/app/structures/stream/stream_listpack"
)

// "ba"/"bc" share a "b" joint node one level deep, while "da" and "fa" sit
// on their own branches with no shared prefix at all. That mix exercises
// both the shared-prefix recursion and the plain sibling fallback.
func newFloorCeilingFixture() (root *RaxNode, lp map[string]*streamlistpack.StreamListpack) {
	root = New(nil, nil)
	lp = map[string]*streamlistpack.StreamListpack{
		"ba": streamlistpack.New([]string{"field", "value"}, 0, 0),
		"bc": streamlistpack.New([]string{"field", "value"}, 0, 0),
		"da": streamlistpack.New([]string{"field", "value"}, 0, 0),
		"fa": streamlistpack.New([]string{"field", "value"}, 0, 0),
	}
	for k, v := range lp {
		root.Insert([]byte(k), v)
	}
	return root, lp
}

func assertFindResult(t *testing.T, got *streamlistpack.StreamListpack, want string, lp map[string]*streamlistpack.StreamListpack) {
	t.Helper()

	if want == "" {
		if got != nil {
			t.Errorf("got a listpack, want nil")
		}
		return
	}
	if got == nil {
		t.Fatalf("got nil, want the listpack inserted for %q", want)
	}
	if got != lp[want] {
		t.Errorf("got the wrong listpack, want the one inserted for %q", want)
	}
}

func TestFindPredRax_Floor(t *testing.T) {
	root, lp := newFloorCeilingFixture()

	tests := []struct {
		name string
		id   string
		want string // key in lp, or "" for nil
	}{
		{"exact match ba", "ba", "ba"},
		{"between ba and bc, shares the b joint", "bb", "ba"},
		{"exact match bc", "bc", "bc"},
		{"between bc and da", "bd", "bc"},
		{"between bc and da, different first byte", "ca", "bc"},
		{"exact match da", "da", "da"},
		{"between da and fa", "ea", "da"},
		{"exact match fa", "fa", "fa"},
		{"above everything", "fb", "fa"},
		{"below everything", "aa", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := root.FindPredRax([]byte(tt.id))
			assertFindResult(t, got, tt.want, lp)
		})
	}
}

func TestFindSucRax_Ceiling(t *testing.T) {
	root, lp := newFloorCeilingFixture()

	tests := []struct {
		name string
		id   string
		want string
	}{
		{"exact match ba", "ba", "ba"},
		{"between ba and bc, shares the b joint", "bb", "bc"},
		{"exact match bc", "bc", "bc"},
		{"between bc and da", "bd", "da"},
		{"between bc and da, different first byte", "ca", "da"},
		{"exact match da", "da", "da"},
		{"between da and fa, no shared prefix", "db", "fa"},
		{"between da and fa", "ea", "fa"},
		{"exact match fa", "fa", "fa"},
		{"below everything", "aa", "ba"},
		{"above everything", "fb", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := root.FindSucRax([]byte(tt.id))
			assertFindResult(t, got, tt.want, lp)
		})
	}
}
