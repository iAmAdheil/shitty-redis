package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/structures/list"
	"github.com/codecrafters-io/redis-starter-go/app/structures/stream"
)

func (com *Com) ping() ([]byte, error) {
	return RESPEncoder("PONG", Simple), nil
}

func (com *Com) echo() ([]byte, error) {
	return RESPEncoder(com.Args["echothis"][0], Bulk), nil
}

func (com *Com) handleType() ([]byte, error) {
	key := com.Args["key"][0]

	kmu.RLock()
	defer kmu.RUnlock()

	e, ok := keyspace[key]
	if ok {
		switch e.Type {
		case TypeVar:
			return RESPEncoder("string", Simple), nil
		case TypeList:
			return RESPEncoder("list", Simple), nil
		case TypeStream:
			return RESPEncoder("stream", Simple), nil
		}
	}

	return RESPEncoder("none", Simple), nil
}

func (com *Com) get() ([]byte, error) {
	key := com.Args["key"][0]

	kmu.RLock()
	defer kmu.RUnlock()

	e, ok := keyspace[key]
	// DNE
	if !ok {
		return RESPEncoder(nil, NullBulk), nil
	}

	v, ok := e.Data.(*string)
	if !ok || e.Type != TypeVar {
		// do something
	}

	return RESPEncoder(*v, Bulk), nil
}

func (com *Com) set() ([]byte, error) {
	key := com.Args["key"][0]
	val := com.Args["value"][0]

	kmu.Lock()
	keyspace[key] = NewStructure(TypeVar, &val)
	kmu.Unlock()

	if len(com.Extras) > 0 {
		expiryType := com.Extras["expiryType"][0]
		dur := com.Extras["duration"][0]

		err := SetupExpiry(expiryType, dur, key)
		if err != nil {
			fmt.Printf("Error setting up expiry for key (%s): %s\n", key, err.Error())
		}
	}

	return RESPEncoder("OK", Simple), nil
}

func (com *Com) rpush() ([]byte, error) {
	key := com.Args["key"][0]
	values := com.Args["values"] // array of values

	kmu.Lock()
	defer kmu.Unlock()

	e, ok := keyspace[key]
	// DNE
	if !ok {
		e = NewStructure(TypeList, list.New())
		keyspace[key] = e
	}

	lp, ok := e.Data.(*list.List)
	if e.Type != TypeList || !ok {
		// do something
	}

	lp.RPUSH(values)
	// calculate before listeners change change len
	len := lp.LLEN()

	handleBLPOPListeners(lp, key)

	return RESPEncoder(len, Int), nil
}

func (com *Com) lrange() ([]byte, error) {
	key := com.Args["key"][0]
	ls := com.Args["left"][0]
	rs := com.Args["right"][0]

	l, err := strconv.Atoi(ls)
	if err != nil {
		fmt.Printf("Error parsing the start index into an integer: %s\n", err.Error())
	}
	r, err := strconv.Atoi(rs)
	if err != nil {
		fmt.Printf("Error parsing the stop index into an integer: %s\n", err.Error())
	}

	var elements []string

	kmu.RLock()
	defer kmu.RUnlock()

	e, ok := keyspace[key]
	// DNE
	if !ok {
		return RESPEncoder(elements, BulkList), nil
	}

	lp, ok := e.Data.(*list.List)
	if e.Type != TypeList || !ok {
		// do something
	}

	elements = lp.LRANGE(l, r)

	return RESPEncoder(elements, BulkList), nil
}

func (com *Com) lpush() ([]byte, error) {
	key := com.Args["key"][0]
	values := com.Args["values"] // array of values

	kmu.Lock()
	defer kmu.Unlock()

	e, ok := keyspace[key]
	// DNE
	if !ok {
		e = NewStructure(TypeList, list.New())
		keyspace[key] = e
	}

	lp, ok := e.Data.(*list.List)
	if e.Type != TypeList || !ok {
		// do something
	}

	lp.LPUSH(values)

	handleBLPOPListeners(lp, key)

	return RESPEncoder(lp.LLEN(), Int), nil
}

func (com *Com) llen() ([]byte, error) {
	key := com.Args["key"][0]

	kmu.RLock()
	defer kmu.RUnlock()

	e, ok := keyspace[key]
	// DNE
	if !ok {
		return RESPEncoder(0, Int), nil
	}

	lp, ok := e.Data.(*list.List)
	if e.Type != TypeList || !ok {
		// do something
	}

	return RESPEncoder(lp.LLEN(), Int), nil
}

func (com *Com) lpop() ([]byte, error) {
	key := com.Args["key"][0]
	c, err := strconv.Atoi(com.Args["count"][0])
	if err != nil {
		// do something
	}

	out := []string{}

	kmu.Lock()
	defer kmu.Unlock()

	e, ok := keyspace[key]
	// DNE
	if !ok {
		return RESPEncoder(nil, NullBulk), nil
	}

	lp, ok := e.Data.(*list.List)
	if e.Type != TypeList || !ok {
		// do something
	}

	out = lp.LPOP(c)

	// delete key when list is empty, 0 waiting listeners
	if len(blpopWaiters[key]) == 0 && lp.LLEN() == 0 {
		delete(keyspace, key)
	}

	if len(out) == 0 {
		return RESPEncoder(nil, NullBulk), nil
	} else if len(out) > 1 {
		// bulk list
		return RESPEncoder(out, BulkList), nil
	}
	// len = 1 -> single element popped -> bulk string
	return RESPEncoder(out[0], Bulk), nil
}

func (com *Com) blpop() ([]byte, error) {
	key := com.Args["key"][0]
	timeout, _ := strconv.ParseFloat(com.Args["timeout"][0], 32)

	var (
		ch  chan string
		out []string = []string{}
		res []byte
	)

	// key -> first part of RESP response
	out = append(out, key)

	kmu.Lock()
	e, ok := keyspace[key]
	// DNE
	if !ok {
		ch = make(chan string)
		// create a new entry and push the listener
		blpopWaiters[key] = append(blpopWaiters[key], ch)

	} else {
		lp, ok := e.Data.(*list.List)
		if e.Type != TypeList || !ok {
			// do something
		}

		if lp.LLEN() > 0 {
			element := lp.LPOP(1)
			out = append(out, element...)
			res = RESPEncoder(out, BulkList)
		} else {
			ch = make(chan string)
			blpopWaiters[key] = append(blpopWaiters[key], ch)
		}
	}
	kmu.Unlock()
	// unlock and exit
	if res != nil {
		return res, nil
	}

	var expch <-chan time.Time
	if timeout > 0 {
		// activate chan after interval
		duration := time.Duration(timeout * float64(time.Second))
		expch = time.After(duration)
	}

	select {
	case pop := <-ch:
		out = append(out, pop)
		res = RESPEncoder(out, BulkList)

	// after acquiring lock if channel found and popped -> return -1
	// else value has been passed into channel -> process and return value
	case <-expch:
		kmu.Lock()
		defer kmu.Unlock()

		isExists := findAndPopCh(ch, key, com.Base)
		// channel popped from the list, no value pushed
		if isExists {
			res = RESPEncoder(nil, NullBulkList)
		} else {
			// false -> chan was filled just before timeout -> process and return happy path
			pop := <-ch
			out = append(out, pop)
			res = RESPEncoder(out, BulkList)
		}
	}

	return res, nil
}

func (com *Com) xadd() ([]byte, error) {
	key := com.Args["key"][0]
	id := com.Args["id"][0]
	data := com.Args["data"]

	kmu.Lock()
	defer kmu.Unlock()
	e, ok := keyspace[key]
	// DNE
	if !ok {
		e = NewStructure(TypeStream, stream.New())
		keyspace[key] = e
	}

	st, ok := e.Data.(*stream.Stream)
	if e.Type != TypeStream || !ok {
		return nil, errors.New("A non-stream element with the given key already exists")
	}

	// check for * in id
	p := strings.Split(id, "-")
	if p[0] == "*" || p[1] == "*" {
		// if either part is *, generate id
		gid, err := st.GenerateStreamEntryId(id) // completed/generated id
		if err != nil {
			return nil, err
		}

		id = gid
	} else {
		// if no *, use validate and use the same
		if err := st.ValidateStreamEntryId(id); err != nil {
			return nil, err
		}
	}
	st.XADD(data, id)

	WakeAllXREADListeners(key)

	return RESPEncoder(id, Bulk), nil
}

func (com *Com) xrange() ([]byte, error) {
	key := com.Args["key"][0]
	low := com.Args["low"][0]
	high := com.Args["high"][0]

	kmu.RLock()
	defer kmu.RUnlock()
	e, ok := keyspace[key]
	// DNE
	if !ok {
		e = NewStructure(TypeStream, stream.New())
		keyspace[key] = e
	}

	st, ok := e.Data.(*stream.Stream)
	if e.Type != TypeStream || !ok {
		return nil, errors.New("A non-stream element with the given key already exists")
	}

	entries, err := st.XRANGE(low, high)
	if err != nil {
		return nil, errors.New("XRANGE failed")
	}

	return RESPEncoder(entries, Array), nil
}

func (com *Com) xread() ([]byte, error) {
	keys := com.Args["keys"]
	ids := com.Args["ids"]

	res := []string{}

	kmu.Lock()

	for i, key := range keys {
		var s string

		e, ok := keyspace[key]
		// DNE
		if !ok {
			continue
		}

		st, ok := e.Data.(*stream.Stream)
		if e.Type != TypeStream || !ok {
			kmu.Unlock() // handle early return
			return nil, errors.New("Operation against a key holding the wrong kind of value")
		}

		if ids[i] == "$" {
			ms, seq := st.GetLastId()
			ids[i] = fmt.Sprintf("%d-%d", ms, seq)
		}

		id := ids[i]
		strEntry, err := st.XREAD(id)
		if len(strEntry) == 0 || err != nil {
			continue
		}

		s += "*2\r\n"
		s += fmt.Sprintf("$%d\r\n%s\r\n", len(key), key)
		s += strEntry

		res = append(res, s)
	}

	_, ok := com.Extras["block"] // duration
	// non-blocking path
	// return whatever result is available
	if len(res) > 0 || !ok {
		kmu.Unlock() // unlock before return
		if len(res) == 0 {
			return RESPEncoder(nil, NullBulkList), nil
		}
		return RESPEncoder(res, Array), nil
	}
	// blocking path
	ch := make(chan string, 1)
	var expch <-chan time.Time

	timeout, err := strconv.ParseFloat(com.Extras["block"][0], 64)
	if err != nil {
		kmu.Unlock() // handle early return
		return nil, errors.New("Invalid XREAD block timeout")
	}

	for _, key := range keys {
		if _, ok := xreadWaiters[key]; !ok {
			xreadWaiters[key] = []chan string{ch}
		} else {
			xreadWaiters[key] = append(xreadWaiters[key], ch)
		}
	}

	kmu.Unlock() // listeners added, release

	if timeout > 0 {
		// activate chan after interval
		duration := time.Duration(timeout * float64(time.Millisecond))
		expch = time.After(duration)
	}

	for {
		select {
		case <-ch: // Transmit Structure key channel
			kmu.Lock()

			for i, key := range keys {
				var s string
				id := ids[i]

				e, ok := keyspace[key]
				// DNE
				if !ok {
					continue
				}

				st, ok := e.Data.(*stream.Stream)
				if e.Type != TypeStream || !ok {
					kmu.Unlock() // handle early return
					return nil, errors.New("Operation against a key holding the wrong kind of value")
				}

				strEntry, err := st.XREAD(id)
				if len(strEntry) == 0 || err != nil {
					continue
				}

				s += "*2\r\n"
				s += fmt.Sprintf("$%d\r\n%s\r\n", len(key), key)
				s += strEntry

				res = append(res, s)
			}

			if len(res) == 0 {
				kmu.Unlock()
				continue
			}

			findAndPopChFromKeys(keys, ch, com.Base)
			kmu.Unlock()

			return RESPEncoder(res, Array), nil

		case <-expch:
			kmu.Lock()
			defer kmu.Unlock()
			findAndPopChFromKeys(keys, ch, com.Base)

			select {
			case <-ch:
				for i, key := range keys {
					var s string
					id := ids[i]

					e, ok := keyspace[key]
					// DNE
					if !ok {
						continue
					}

					st, ok := e.Data.(*stream.Stream)
					if e.Type != TypeStream || !ok {
						kmu.Unlock() // handle early return
						return nil, errors.New("Operation against a key holding the wrong kind of value")
					}

					strEntry, err := st.XREAD(id)
					if len(strEntry) == 0 || err != nil {
						continue
					}

					s += "*2\r\n"
					s += fmt.Sprintf("$%d\r\n%s\r\n", len(key), key)
					s += strEntry

					res = append(res, s)
				}

				if len(res) == 0 {
					return RESPEncoder(nil, NullBulkList), nil
				}
				return RESPEncoder(res, Array), nil

			default: // null ch
				return RESPEncoder(nil, NullBulkList), nil
			}
		}
	}
}
