package stream

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func splitId(id string) (uint64, uint64, error) {
	nums := strings.Split(id, "-")
	m, s := nums[0], nums[1]

	ms, err := strconv.ParseUint(m, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	seq, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return ms, seq, nil
}

func (st *Stream) ValidateStreamEntryId(id string) error {
	ms, seq, err := splitId(id)
	if err != nil {
		return err
	}

	if ms == 0 && seq == 0 {
		return errors.New("The ID specified in XADD must be greater than 0-0")
	}

	if st != nil && st.LastId != nil {
		msEnd, seqEnd := st.LastId.ms, st.LastId.seq
		if ms < msEnd || (ms == msEnd && seq <= seqEnd) {
			return errors.New("The ID specified in XADD is equal or smaller than the target stream top item")
		}
	}

	return nil
}

func (st *Stream) GenerateStreamEntryId(id string) (string, error) {
	p := strings.Split(id, "-")
	switch "*" {
	case p[0]:
		ms := uint64(time.Now().UnixMilli())
		if st == nil || st.LastId == nil {
			return fmt.Sprintf("%d-0", ms), nil
		}
		lastMs, lastSeq := st.LastId.ms, st.LastId.seq
		if ms > lastMs {
			return fmt.Sprintf("%d-0", ms), nil
		}
		// clock hasn't advanced past the last entry's ms: stay monotonic
		return fmt.Sprintf("%d-%d", lastMs, lastSeq+1), nil
	case p[1]:
		if st == nil || st.LastId == nil {
			msId, err := strconv.ParseUint(p[0], 10, 64)
			if err != nil {
				return "", errors.New("Invalid id")
			}
			seq := uint64(0)
			if msId == 0 {
				seq = 1 // 0-0 is a reserved id
			}
			return fmt.Sprintf("%d-%d", msId, seq), nil
		} else {
			ms, seq := st.LastId.ms, st.LastId.seq

			msId, err := strconv.ParseUint(p[0], 10, 64)
			if err != nil {
				return "", errors.New("Invalid id")
			}

			if msId < ms {
				return "", errors.New("Invalid id")
			} else if msId > ms {
				return fmt.Sprintf("%d-0", msId), nil
			}

			return fmt.Sprintf("%d-%d", ms, seq+1), nil
		}
	default:
		return "", nil
	}
}

// handle ms only -> append seq
func formatXRANGEIds(low, high string) (string, string) {
	pLow := strings.Split(low, "-")
	pHigh := strings.Split(high, "-")

	if len(pLow) == 1 {
		low += "-0"
	}
	if len(pHigh) == 1 {
		high += fmt.Sprintf("-%d", uint64(math.MaxUint64))
	}

	return low, high
}
