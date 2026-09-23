package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/structures/list"
	"github.com/codecrafters-io/redis-starter-go/app/structures/stream"
)

func (com *Com) ping() []byte {
	return RESPEncoder("PONG", Simple)
}

func (com *Com) echo() []byte {
	return RESPEncoder(com.Args["echothis"][0], Bulk)
}

func (com *Com) handleType() []byte {
	key := com.Args["key"][0]

	kmu.RLock()
	defer kmu.RUnlock()

	e, ok := keyspace[key]
	if ok {
		switch e.Type {
		case TypeVar:
			return RESPEncoder("string", Simple)
		case TypeList:
			return RESPEncoder("list", Simple)
		case TypeStream:
			return RESPEncoder("stream", Simple)
		}
	}

	return RESPEncoder("none", Simple)
}

func (com *Com) get() []byte {
	key := com.Args["key"][0]

	kmu.RLock()
	defer kmu.RUnlock()

	e, ok := keyspace[key]
	// DNE
	if !ok {
		return RESPEncoder(nil, NullBulk)
	}

	v, ok := e.Data.(*string)
	if !ok || e.Type != TypeVar {
		// do something
	}

	return RESPEncoder(*v, Bulk)
}

func (com *Com) set() []byte {
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

	return RESPEncoder("OK", Simple)
}

func (com *Com) rpush() []byte {
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

	e.handleListeners(lp)

	return RESPEncoder(lp.LLEN(), Int)
}

func (com *Com) lrange() []byte {
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
		return RESPEncoder(elements, BulkList)
	}

	lp, ok := e.Data.(*list.List)
	if e.Type != TypeList || !ok {
		// do something
	}

	elements = lp.LRANGE(l, r)

	return RESPEncoder(elements, BulkList)
}

func (com *Com) lpush() []byte {
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

	e.handleListeners(lp)

	return RESPEncoder(lp.LLEN(), Int)
}

func (com *Com) llen() []byte {
	key := com.Args["key"][0]

	kmu.RLock()
	defer kmu.RUnlock()

	e, ok := keyspace[key]
	// DNE
	if !ok {
		return RESPEncoder(0, Int)
	}

	lp, ok := e.Data.(*list.List)
	if e.Type != TypeList || !ok {
		// do something
	}

	return RESPEncoder(lp.LLEN(), Int)
}

func (com *Com) lpop() []byte {
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
		return RESPEncoder(nil, NullBulk)
	}

	lp, ok := e.Data.(*list.List)
	if e.Type != TypeList || !ok {
		// do something
	}

	out = lp.LPOP(c)

	// delete key when list is empty, 0 waiting listeners
	if len(e.Listeners) == 0 && lp.LLEN() == 0 {
		delete(keyspace, key)
	}

	if len(out) == 0 {
		return RESPEncoder(nil, NullBulk)
	} else if len(out) > 1 {
		// bulk list
		return RESPEncoder(out, BulkList)
	}
	// len = 1 -> single element popped -> bulk string
	return RESPEncoder(out[0], Bulk)
}

func (com *Com) blpop() []byte {
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
		e = NewStructure(TypeList, list.New())
		keyspace[key] = e
		e.Listeners = append(e.Listeners, ch)

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
			e.Listeners = append(e.Listeners, ch)
		}
	}
	kmu.Unlock()
	// unlock and exit
	if res != nil {
		return res
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

		e, ok := keyspace[key]
		// DNE
		if !ok {
			// something went wrong
			// key got deleted with the client's listener(ch) missing
		}

		isExists := e.findAndPopCh(ch)
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

	return res
}

func (com *Com) xadd() []byte {
	key := com.Args["key"][0]
	id := com.Args["id"][0]
	data := com.Args["data"]

	kmu.RLock()
	e, ok := keyspace[key]
	// DNE
	if !ok {
		e = NewStructure(TypeStream, stream.New())
		keyspace[key] = e
	}

	st, ok := e.Data.(*stream.Stream)
	if e.Type != TypeStream || !ok {
		// do something
	}

	st.XADD(data, id)

	return RESPEncoder(id, Bulk)
}

func (com *Com) xrange() []byte {
	key := com.Args["streamkey"][0]
	low := com.Args["low"][0]
	high := com.Args["high"][0]

	smu.RLock()
	stream, _ := streams[key]
	smu.RUnlock()

	// getEntriesInMilRange -> handles its own stream locking
	entries, err := stream.getEntriesInRange(low, high)
	if err != nil {
		// do something
	}

	return RESPEncoder(entries, Array)
}
