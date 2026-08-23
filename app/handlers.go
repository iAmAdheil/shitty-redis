package main

import (
	"fmt"
	"strconv"
	"time"
)

func (com *Com) ping() []byte {
	return RESPEncoder([]string{"PONG"}, Simple)
}

func (com *Com) echo() []byte {
	s := com.Args["echothis"][0]

	return RESPEncoder([]string{s}, Bulk)
}

func (com *Com) get() []byte {
	key := com.Args["key"][0]

	vmu.Lock()
	val, ok := vars[key]
	defer vmu.Unlock()

	if !ok {
		return RESPEncoder(nil, NullBulk)
	} else {
		return RESPEncoder([]string{val}, Bulk)
	}
}

func (com *Com) set() []byte {
	key := com.Args["key"][0]
	val := com.Args["value"][0]

	vmu.Lock()
	vars[key] = val
	vmu.Unlock()

	// parts has an element at index 8 and index 10
	// if parts has expiry args sent -> only then setup expiry
	if len(com.Extras) > 0 {
		expiryType := com.Extras["expiryType"][0]
		dur := com.Extras["duration"][0]

		err := SetupExpiry(expiryType, dur, key)
		if err != nil {
			fmt.Printf("Error setting up expiry for key (%s): %s\n", key, err.Error())
		}
	}

	return RESPEncoder([]string{"OK"}, Simple)
}

func (com *Com) rpush() []byte {
	listkey := com.Args["listkey"][0]
	values := com.Args["values"] // array of values

	listsize := AddToList(listkey, values, 0)

	return RESPEncoder([]string{strconv.Itoa(listsize)}, Int)
}

func (com *Com) lrange() []byte {
	listkey := com.Args["listkey"][0]
	ls := com.Args["left"][0]
	rs := com.Args["right"][0]

	l, err := strconv.ParseInt(ls, 10, 0)
	if err != nil {
		fmt.Printf("Error parsing the start index into an integer: %s\n", err.Error())
	}
	r, err := strconv.ParseInt(rs, 10, 0)
	if err != nil {
		fmt.Printf("Error parsing the stop index into an integer: %s\n", err.Error())
	}

	out := GetListRange(listkey, int(l), int(r))
	return RESPEncoder(out, BulkList)
}

func (com *Com) lpush() []byte {
	listkey := com.Args["listkey"][0]
	values := com.Args["values"]

	listsize := AddToList(listkey, values, 1)

	out := []string{strconv.Itoa(listsize)}
	return RESPEncoder(out, Int)
}

func (com *Com) llen() []byte {
	listkey := com.Args["listkey"][0]
	listsize := GetListLen(listkey)

	out := []string{strconv.Itoa(listsize)}
	return RESPEncoder(out, Int)
}

func (com *Com) lpop() []byte {
	listkey := com.Args["listkey"][0]
	count, err := strconv.ParseInt(com.Args["count"][0], 10, 0)
	if err != nil {
		// do something
	}

	out := []string{}
	s, err := DeleteFromList(listkey, int(count), 1)
	if err == nil {
		out = append(out, s...)
	}

	if len(out) > 1 {
		// bulk list
		return RESPEncoder(out, BulkList)
	}
	// bulk string
	return RESPEncoder(out, Bulk)
}

func (com *Com) blpop() []byte {
	listkey := com.Args["listkey"][0]
	ts := com.Args["timeout"][0]

	timeout, _ := strconv.ParseFloat(ts, 32)

	var ch chan string

	out := []string{}
	out = append(out, listkey)

	lmu.Lock()
	l, ok := lists[listkey]

	if !ok || len(*l) == 0 {
		ch = make(chan string, 1)

		lch, ok := listch[listkey]
		if !ok {
			lch = []chan string{ch}
		} else {
			lch = append(lch, ch)
		}

		listch[listkey] = lch

	} else if len(*l) >= 1 {
		s, err := DeleteFromList(listkey, 1, 1)
		if err == nil {
			out = append(out, s...)
		}

		// only reach this if list has elements available to pop
		// imp. to release resource
		lmu.Unlock()

		return RESPEncoder(out, BulkList)
	}

	lmu.Unlock()

	var expch <-chan time.Time
	if timeout > 0 {
		// activate chan after interval
		duration := time.Duration(timeout * float64(time.Second))
		expch = time.After(duration)
	}

	var res []byte

	select {
	case pop := <-ch:
		out = append(out, pop)
		res = RESPEncoder(out, BulkList)

	// lock to update channel slice
	// after acquiring lock if channel found and popped -> return -1
	// else value has been passed into channel -> process and return value
	case <-expch:
		lmu.Lock()
		// check if chan is still in array
		// true -> pop chan and return -1
		lch := listch[listkey]
		// handles nil channel list -> no need to check for ok
		ulch, isExists := popChan(lch, ch)
		// channel popped from the list, no value pushed
		if isExists {
			listch[listkey] = ulch
			res = RESPEncoder(nil, NullBulkList)
		} else {
			// false -> chan was filled just before timeout -> process and return happy path
			pop := <-ch
			out = append(out, pop)
			res = RESPEncoder(out, BulkList)
		}

		lmu.Unlock()
	}

	return res
}

func (com *Com) handleType() []byte {
	key := com.Args["key"][0]
	var out []string

	vmu.Lock()
	val, ok := vars[key]
	if !ok {
		out = []string{"none"}
	} else {
		t := fmt.Sprintf("%T", val)
		out = []string{t}
	}
	defer vmu.Unlock()

	return RESPEncoder(out, Simple)
}
