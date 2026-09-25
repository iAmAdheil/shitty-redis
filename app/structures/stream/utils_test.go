package stream

import (
	"fmt"
	"testing"
	"time"
)

// safeValidate calls ValidateStreamEntryId and converts a panic into a
// reported flag instead of crashing the whole test binary. st != nil does
// not imply st.LastId != nil (see New()), and utils.go currently checks
// st == nil instead of st.LastId == nil, so an empty stream can panic here.
func safeValidate(t *testing.T, st *Stream, id string) (err error, panicked bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			t.Logf("ValidateStreamEntryId(%q) panicked: %v", id, r)
		}
	}()
	err = st.ValidateStreamEntryId(id)
	return
}

// safeGenerate is the GenerateStreamEntryId equivalent of safeValidate.
func safeGenerate(t *testing.T, st *Stream, id string) (out string, err error, panicked bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			t.Logf("GenerateStreamEntryId(%q) panicked: %v", id, r)
		}
	}()
	out, err = st.GenerateStreamEntryId(id)
	return
}

func TestValidateStreamEntryId_FreshStream_AcceptsPositiveId(t *testing.T) {
	st := New()

	err, panicked := safeValidate(t, st, "1-1")
	if panicked {
		t.Fatalf("ValidateStreamEntryId(%q) on a fresh stream panicked, want a nil error", "1-1")
	}
	if err != nil {
		t.Errorf("ValidateStreamEntryId(%q) = %v, want nil error", "1-1", err)
	}
}

func TestValidateStreamEntryId_FreshStream_RejectsZeroZero(t *testing.T) {
	st := New()

	err, panicked := safeValidate(t, st, "0-0")
	if panicked {
		t.Fatalf("ValidateStreamEntryId(%q) on a fresh stream panicked, want an error", "0-0")
	}
	if err == nil {
		t.Errorf("ValidateStreamEntryId(%q) = nil, want an error: 0-0 is reserved", "0-0")
	}
}

func TestValidateStreamEntryId_ExistingStream_AcceptsGreaterId(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"greater ms", "6-0"},
		{"same ms, greater seq", "5-6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := New()
			st.XADD([]string{"field", "value"}, "5-5")

			err, panicked := safeValidate(t, st, tt.id)
			if panicked {
				t.Fatalf("ValidateStreamEntryId(%q) panicked, want a nil error", tt.id)
			}
			if err != nil {
				t.Errorf("ValidateStreamEntryId(%q) = %v, want nil error", tt.id, err)
			}
		})
	}
}

func TestValidateStreamEntryId_ExistingStream_RejectsLowerOrEqualId(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"equal id", "5-5"},
		{"lower ms", "4-9"},
		{"same ms, lower seq", "5-4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := New()
			st.XADD([]string{"field", "value"}, "5-5")

			err, panicked := safeValidate(t, st, tt.id)
			if panicked {
				t.Fatalf("ValidateStreamEntryId(%q) panicked, want an error", tt.id)
			}
			if err == nil {
				t.Errorf("ValidateStreamEntryId(%q) = nil, want an error: id must be strictly greater than the last id (5-5)", tt.id)
			}
		})
	}
}

func TestValidateStreamEntryId_MalformedId_ReturnsErrorNotPanic(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"non-numeric ms", "abc-1"},
		{"non-numeric seq", "1-abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := New()
			st.XADD([]string{"field", "value"}, "5-5")

			err, panicked := safeValidate(t, st, tt.id)
			if panicked {
				t.Fatalf("ValidateStreamEntryId(%q) panicked, want an error", tt.id)
			}
			if err == nil {
				t.Errorf("ValidateStreamEntryId(%q) = nil, want an error", tt.id)
			}
		})
	}
}

func TestGenerateStreamEntryId_FreshStream_FullyAuto_UsesCurrentTimestamp(t *testing.T) {
	st := New()

	before := time.Now().UnixMilli()
	id, err, panicked := safeGenerate(t, st, "*")
	after := time.Now().UnixMilli()
	if panicked {
		t.Fatalf("GenerateStreamEntryId(%q) on a fresh stream panicked", "*")
	}
	if err != nil {
		t.Fatalf("GenerateStreamEntryId(%q) error = %v, want nil", "*", err)
	}

	ms, seq, err := splitId(id)
	if err != nil {
		t.Fatalf("GenerateStreamEntryId(%q) = %q, not a valid id: %v", "*", id, err)
	}
	if int64(ms) < before || int64(ms) > after {
		t.Errorf("GenerateStreamEntryId(%q) ms = %d, want between %d and %d (the current Unix ms timestamp)", "*", ms, before, after)
	}
	if seq != 0 {
		t.Errorf("GenerateStreamEntryId(%q) seq = %d, want 0: first entry at this ms", "*", seq)
	}
}

func TestGenerateStreamEntryId_ExistingStream_FullyAuto_SameMsIncrementsSeq(t *testing.T) {
	st := New()
	now := uint64(time.Now().UnixMilli())
	st.XADD([]string{"field", "value"}, fmt.Sprintf("%d-5", now))

	id, err, panicked := safeGenerate(t, st, "*")
	if panicked {
		t.Fatalf("GenerateStreamEntryId(%q) panicked", "*")
	}
	if err != nil {
		t.Fatalf("GenerateStreamEntryId(%q) error = %v, want nil", "*", err)
	}

	ms, seq, err := splitId(id)
	if err != nil {
		t.Fatalf("GenerateStreamEntryId(%q) = %q, not a valid id: %v", "*", id, err)
	}
	switch {
	case ms == now && seq != 6:
		t.Errorf("GenerateStreamEntryId(%q) = %d-%d, want seq 6: the clock hasn't advanced past the last entry's ms", "*", ms, seq)
	case ms > now && seq != 0:
		t.Errorf("GenerateStreamEntryId(%q) = %d-%d, want seq 0: the clock advanced to a new ms", "*", ms, seq)
	case ms < now:
		t.Errorf("GenerateStreamEntryId(%q) ms = %d, want >= %d: id must never go backward", "*", ms, now)
	}
}

func TestGenerateStreamEntryId_FreshStream_MsGiven_SeqAuto(t *testing.T) {
	st := New()

	id, err, panicked := safeGenerate(t, st, "5-*")
	if panicked {
		t.Fatalf("GenerateStreamEntryId(%q) on a fresh stream panicked, want %q", "5-*", "5-0")
	}
	if err != nil {
		t.Fatalf("GenerateStreamEntryId(%q) error = %v, want nil", "5-*", err)
	}
	if id != "5-0" {
		t.Errorf("GenerateStreamEntryId(%q) = %q, want %q", "5-*", id, "5-0")
	}
}

func TestGenerateStreamEntryId_ExistingStream_MsGiven_SeqAuto(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		want    string
		wantErr bool
	}{
		{"same ms increments seq", "5-*", "5-6", false},
		{"greater ms resets seq to 0", "6-*", "6-0", false},
		{"lower ms is an error", "4-*", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := New()
			st.XADD([]string{"field", "value"}, "5-5")

			id, err, panicked := safeGenerate(t, st, tt.id)
			if panicked {
				t.Fatalf("GenerateStreamEntryId(%q) panicked", tt.id)
			}
			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateStreamEntryId(%q) = %q, nil, want an error", tt.id, id)
				}
				return
			}
			if err != nil {
				t.Fatalf("GenerateStreamEntryId(%q) error = %v, want nil", tt.id, err)
			}
			if id != tt.want {
				t.Errorf("GenerateStreamEntryId(%q) = %q, want %q", tt.id, id, tt.want)
			}
		})
	}
}

func TestGenerateStreamEntryId_MalformedMs_ReturnsErrorNotPanic(t *testing.T) {
	st := New()
	st.XADD([]string{"field", "value"}, "5-5")

	id, err, panicked := safeGenerate(t, st, "abc-*")
	if panicked {
		t.Fatalf("GenerateStreamEntryId(%q) panicked, want an error", "abc-*")
	}
	if err == nil {
		t.Errorf("GenerateStreamEntryId(%q) = %q, nil, want an error", "abc-*", id)
	}
}

func TestGenerateStreamEntryId_NoWildcard_ReturnsEmptyResult(t *testing.T) {
	st := New()
	st.XADD([]string{"field", "value"}, "5-5")

	id, err, panicked := safeGenerate(t, st, "5-5")
	if panicked {
		t.Fatalf("GenerateStreamEntryId(%q) panicked", "5-5")
	}
	if err != nil {
		t.Errorf("GenerateStreamEntryId(%q) error = %v, want nil", "5-5", err)
	}
	if id != "" {
		t.Errorf("GenerateStreamEntryId(%q) = %q, want empty string (no wildcard to generate)", "5-5", id)
	}
}

func TestGenerateStreamEntryId_FreshStream_MsZero_SeqStartsAtOne(t *testing.T) {
	st := New()

	id, err, panicked := safeGenerate(t, st, "0-*")
	if panicked {
		t.Fatalf("GenerateStreamEntryId(%q) on a fresh stream panicked, want %q", "0-*", "0-1")
	}
	if err != nil {
		t.Fatalf("GenerateStreamEntryId(%q) error = %v, want nil", "0-*", err)
	}
	if id != "0-1" {
		t.Errorf("GenerateStreamEntryId(%q) = %q, want %q: 0-0 is a reserved id", "0-*", id, "0-1")
	}
}
