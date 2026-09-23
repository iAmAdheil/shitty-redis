package stream

import (
	"strconv"
	"strings"
)

func splitId(id string) (uint64, uint64) {
	nums := strings.Split(id, "-")
	m, s := nums[0], nums[1]

	ms, err := strconv.ParseUint(m, 10, 64)
	if err != nil {
		// do something
	}
	seq, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		// do something
	}

	return ms, seq
}
