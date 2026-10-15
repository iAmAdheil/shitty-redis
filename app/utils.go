package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/structures/list"
)

// # Imp points (@iAmAdheil) -> pls take a look later
// - arg validation should happen during decoding -> string to int conversions should not happen within my com handlers

func handleBLPOPListeners(lp *list.List, key string) {
	var pop int

	for _, ch := range blpopWaiters[key] {
		if lp.LLEN() == 0 {
			break
		}

		// pop a single element and inject into channel
		element := lp.LPOP(1)[0]
		ch <- element
		pop++
	}

	blpopWaiters[key] = blpopWaiters[key][pop:]
}

func findAndPopChFromKeys(keys []string, target chan string, com string) {
	for _, key := range keys {
		findAndPopCh(target, key, com)
	}
}

func findAndPopCh(target chan string, key string, com string) bool {
	var (
		clone    []chan string
		isExists = false
	)

	switch com {
	case "blpop":
		for _, ch := range blpopWaiters[key] {
			if ch != target {
				clone = append(clone, ch)
			} else {
				isExists = true
			}
		}

		blpopWaiters[key] = clone

	case "xread":
		for _, ch := range xreadWaiters[key] {
			if ch != target {
				clone = append(clone, ch)
			} else {
				isExists = true
			}
		}

		xreadWaiters[key] = clone

	}

	return isExists
}

func SetupExpiry(et string, dur string, key string) error {
	// et -> expiry type
	// ex -> second
	// px -> millisecond
	var m time.Duration

	if strings.ToLower(et) == "px" {
		m = time.Millisecond
	} else if strings.ToLower(et) == "ex" {
		m = time.Second
	} else {
		return errors.New("Unknown expiry type")
	}

	d, err := strconv.ParseInt(dur, 10, 64)
	if err != nil {
		return fmt.Errorf("Error parsing the duration into an integer: %s\n", err.Error())
	}

	go Expire(time.Duration(d)*m, key)
	return nil
}

func Expire(t time.Duration, key string) {
	time.Sleep(t)

	kmu.Lock()
	delete(keyspace, key)
	kmu.Unlock()
}
