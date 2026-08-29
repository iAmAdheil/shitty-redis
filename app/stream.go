package main

import (
	"errors"
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

func (st *Stream) getMaxEntry() (int64, int, error) {
	var maxMil int64 = 0
	var maxI int = 0

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

	return maxMil, maxI, nil
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

func generateStreamEntryId(key, id string) (string, error) {
	var gid string
	p := strings.Split(id, "-")

	// full id
	if p[0] == "*" {
		milInt := time.Now().UnixMilli()
		mil := strconv.FormatInt(milInt, 10)

		stream, ok := streams[key]
		if !ok {
			if mil == "0" {
				gid = mil + "-" + "1"
			} else {
				gid = mil + "-" + "0"
			}
		} else {
			maxMil, maxI, err := stream.getMaxEntry()
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

	} else if p[1] == "*" {
		// partial id
		stream, ok := streams[key]
		if !ok || len(*(stream.entries)) == 0 {
			if p[0] == "0" {
				gid = p[0] + "-" + "1"
			} else {
				gid = p[0] + "-" + "0"
			}
		} else {
			mil, err := strconv.ParseInt(p[0], 10, 64)
			if err != nil {
				return "", err
			}

			maxMil, maxI, err := stream.getMaxEntry()
			if err != nil {
				return "", err
			}

			if maxMil < mil {
				gid = p[0] + "-" + "0"
			} else if maxMil == mil {
				gid = p[0] + "-" + strconv.Itoa(maxI+1)
			} else {
				return "", errors.New("The ID specified in XADD is smaller than the target stream top item")
			}
		}
	}

	return gid, nil
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

	stream, ok := streams[key]
	// any id other than 0-0 is valid
	if !ok || len(*(stream.entries)) == 0 {
		return nil
	}

	stream.mu.RLock()
	defer stream.mu.RUnlock()
	// get max id
	maxMil, maxI, err := stream.getMaxEntry()
	if err != nil {
		return err
	}

	if mil < maxMil || (mil == maxMil && i <= maxI) {
		return errors.New("The ID specified in XADD is equal or smaller than the target stream top item")
	}

	return nil
}
