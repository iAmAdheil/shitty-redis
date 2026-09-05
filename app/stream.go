package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// streamkey:{lock, entries: {id: {key: value}}}
var streams = make(map[string]*Stream)
var smu = sync.RWMutex{}

type Stream struct {
	mu      *sync.RWMutex
	entries *map[string]map[string]string
}

// note -> depends on caller to lock stream, because of conditional
func (st *Stream) getMaxEntry(premil int64) (int64, int, error) {
	var maxMil int64 = 0
	var maxI int = 0

	// if -1 -> find greatest entry id
	switch premil {
	case -1:
		for k, _ := range *(st.entries) {
			mil, i, err := splitStreamEntryId(k)
			if err != nil {
				return 0, 0, err
			}

			if mil > maxMil || (mil == maxMil && i > maxI) {
				maxMil = mil
				maxI = i
			}
		}
	// else -> greatest entry id with given mil value
	default:
		maxMil = premil
		for k, _ := range *(st.entries) {
			mil, i, err := splitStreamEntryId(k)
			if err != nil {
				return 0, 0, err
			}

			if mil == maxMil && i > maxI {
				maxI = i
			}
		}
	}

	return maxMil, maxI, nil
}

// unformatted low and high boundaries
func formatBoundXRange(stream *Stream, unflow, unfhigh string) (string, string, error) {
	var (
		low, high string
	)

	// add 0 to lower boundary
	pl := strings.Split(unflow, "-")
	switch len(pl) {
	case 2:
		low = unflow
	case 1:
		low = unflow + "0"
	default:
		return "", "", errors.New("Invalid XRange boundary")
	}

	// add largest seq number for the given mil value
	ph := strings.Split(unfhigh, "-")
	switch len(ph) {
	case 2:
		high = unfhigh
	case 1:
		premil, err := strconv.ParseInt(pl[0], 10, 64)
		if err != nil {
			return "", "", errors.New("Invalid XRange boundary")
		}

		_, maxI, err := stream.getMaxEntry(premil)
		if err != nil {
			return "", "", errors.New("Invalid XRange boundary")
		}
		high = unfhigh + strconv.Itoa(maxI)
	default:
		return "", "", errors.New("Invalid XRange boundary")
	}

	return low, high, nil
}

func (st *Stream) getEntriesInRange(unflow, unfhigh string) ([]string, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	res := []string{}

	// format boundaries into valid ids
	low, high, err := formatBoundXRange(st, unflow, unfhigh)
	if err != nil {
		return res, errors.New("Invalid XRange boundary")
	}

	lowMil, lowI, err := splitStreamEntryId(low)
	if err != nil {
		return res, errors.New("Invalid XRange boundary")
	}
	highMil, highI, err := splitStreamEntryId(high)
	if err != nil {
		return res, errors.New("Invalid XRange boundary")
	}

	for id, pairs := range *(st.entries) {
		mil, i, err := splitStreamEntryId(id)
		if err != nil {
			return res, err
		}

		if (lowMil < mil && mil < highMil) || (lowMil == mil && highMil != mil && i >= lowI) || (lowMil != mil && highMil == mil && i <= highI) || (lowMil == mil && highMil == mil && lowI <= i && i <= highI) {
			var s string = "*2\r\n"
			s += fmt.Sprintf("$%d\r\n%s\r\n", len(id), id)

			pairCount := len(pairs)
			s += fmt.Sprintf("*%d\r\n", pairCount*2)

			for key, val := range pairs {
				s += fmt.Sprintf("$%d\r\n%s\r\n", len(key), key)
				s += fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)
			}

			res = append(res, s)
		}
	}

	return res, nil
}

func splitStreamEntryId(id string) (int64, int, error) {
	p := strings.Split(id, "-")
	if len(p) != 2 {
		return 0, 0, errors.New("Invalid stream entry Id")
	}

	mil, err := strconv.ParseInt(p[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	i, err := strconv.Atoi(p[1])
	if err != nil {
		return 0, 0, err
	}

	return mil, i, nil
}

func validateStreamEntryId(key, id string) error {
	mil, i, err := splitStreamEntryId(id)
	if err != nil {
		return err
	}

	// check for invalid stream entry id 0-0
	if mil == 0 && i == 0 {
		return errors.New("The ID specified in XADD must be greater than 0-0")
	}

	smu.RLock()
	stream, ok := streams[key]
	// release as soon as pointer to stream acquired
	smu.RUnlock()
	// any id other than 0-0 is valid
	if !ok {
		return nil
	}

	stream.mu.RLock()
	defer stream.mu.RUnlock()

	if len(*(stream.entries)) == 0 {
		return nil
	}

	// get max id
	maxMil, maxI, err := stream.getMaxEntry(-1)
	if err != nil {
		return err
	}

	if mil < maxMil || (mil == maxMil && i <= maxI) {
		return errors.New("The ID specified in XADD is equal or smaller than the target stream top item")
	}

	return nil
}

func generateStreamEntryId(key, id string) (string, error) {
	p := strings.Split(id, "-")

	// full id
	if p[0] == "*" {
		return handleGenerateFullId(key)
	} else if p[1] == "*" {
		// partial id
		return handleGeneratePartialId(key, p[0])
	}

	return "", nil
}

func handleGenerateFullId(key string) (string, error) {
	var gid string

	milInt := time.Now().UnixMilli()
	mil := strconv.FormatInt(milInt, 10)

	smu.RLock()
	stream, ok := streams[key]
	smu.RUnlock()
	if !ok {
		if mil == "0" {
			gid = mil + "-" + "1"
		} else {
			gid = mil + "-" + "0"
		}
	} else {
		stream.mu.RLock()
		defer stream.mu.RUnlock()
		maxMil, maxI, err := stream.getMaxEntry(-1)
		if err != nil {
			return "", err
		}

		if maxMil < milInt {
			gid = mil + "-" + "0"
		} else if maxMil == milInt {
			gid = mil + "-" + strconv.Itoa(maxI+1)
		} else {
			return "", errors.New("The ID specified in XADD is smaller than the target stream top item")
		}
	}

	return gid, nil
}

// mils -> string mil extracted from the first part of the entry id
// passed from the caller
func handleGeneratePartialId(key string, mils string) (string, error) {
	var gid string

	smu.RLock()
	stream, ok := streams[key]
	smu.RUnlock()

	if !ok {
		if mils == "0" {
			gid = mils + "-" + "1"
		} else {
			gid = mils + "-" + "0"
		}

		return gid, nil
	}

	stream.mu.RLock()
	defer stream.mu.RUnlock()
	if len(*(stream.entries)) == 0 {
		if mils == "0" {
			gid = mils + "-" + "1"
		} else {
			gid = mils + "-" + "0"
		}
	} else {
		mil, err := strconv.ParseInt(mils, 10, 64)
		if err != nil {
			return "", err
		}
		maxMil, maxI, err := stream.getMaxEntry(-1)
		if err != nil {
			return "", err
		}

		if maxMil < mil {
			gid = mils + "-" + "0"
		} else if maxMil == mil {
			gid = mils + "-" + strconv.Itoa(maxI+1)
		} else {
			return "", errors.New("The ID specified in XADD is smaller than the target stream top item")
		}
	}

	return gid, nil
}
