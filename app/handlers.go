package main

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
	"github.com/codecrafters-io/redis-starter-go/app/server"
	"github.com/codecrafters-io/redis-starter-go/app/structures/list"
	"github.com/codecrafters-io/redis-starter-go/app/structures/stream"
)

func (com *Com) ping() ([]byte, error) {
	return resp.RESPEncoder("PONG", resp.Simple), nil
}

func (com *Com) echo() ([]byte, error) {
	return resp.RESPEncoder(com.Args["echothis"][0], resp.Bulk), nil
}

func (com *Com) handleType() ([]byte, error) {
	key := com.Args["key"][0]

	kmu.RLock()
	defer kmu.RUnlock()

	e, ok := keyspace[key]
	if ok {
		switch e.Type {
		case TypeVar:
			return resp.RESPEncoder("string", resp.Simple), nil
		case TypeList:
			return resp.RESPEncoder("list", resp.Simple), nil
		case TypeStream:
			return resp.RESPEncoder("stream", resp.Simple), nil
		}
	}

	return resp.RESPEncoder("none", resp.Simple), nil
}

func (com *Com) get() ([]byte, error) {
	key := com.Args["key"][0]

	kmu.RLock()
	defer kmu.RUnlock()

	e, ok := keyspace[key]
	// DNE
	if !ok {
		return resp.RESPEncoder(nil, resp.NullBulk), nil
	}

	v, ok := e.Data.(*string)
	if !ok || e.Type != TypeVar {
		// do something
	}

	return resp.RESPEncoder(*v, resp.Bulk), nil
}

func (com *Com) set() ([]byte, error) {
	key := com.Args["key"][0]
	val := com.Args["value"][0]

	kmu.Lock()
	keyspace[key] = NewStructure(TypeVar, &val)
	SetWatchingTxnsDirty(key)
	kmu.Unlock()

	if len(com.Extras) > 0 {
		expiryType := com.Extras["expiryType"][0]
		dur := com.Extras["duration"][0]

		err := SetupExpiry(expiryType, dur, key)
		if err != nil {
			fmt.Printf("Error setting up expiry for key (%s): %s\n", key, err.Error())
		}
	}

	return resp.RESPEncoder("OK", resp.Simple), nil
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
	if lp.GetCount() == 0 {
		DeleteStructure(key)
	}

	return resp.RESPEncoder(len, resp.Int), nil
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
		return resp.RESPEncoder(elements, resp.BulkList), nil
	}

	lp, ok := e.Data.(*list.List)
	if e.Type != TypeList || !ok {
		// do something
	}

	elements = lp.LRANGE(l, r)

	return resp.RESPEncoder(elements, resp.BulkList), nil
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
	len := lp.LLEN()

	handleBLPOPListeners(lp, key)
	if lp.GetCount() == 0 {
		DeleteStructure(key)
	}

	return resp.RESPEncoder(len, resp.Int), nil
}

func (com *Com) llen() ([]byte, error) {
	key := com.Args["key"][0]

	kmu.RLock()
	defer kmu.RUnlock()

	e, ok := keyspace[key]
	// DNE
	if !ok {
		return resp.RESPEncoder(0, resp.Int), nil
	}

	lp, ok := e.Data.(*list.List)
	if e.Type != TypeList || !ok {
		// do something
	}

	return resp.RESPEncoder(lp.LLEN(), resp.Int), nil
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
		return resp.RESPEncoder(nil, resp.NullBulk), nil
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
		return resp.RESPEncoder(nil, resp.NullBulk), nil
	} else if len(out) > 1 {
		// bulk list
		return resp.RESPEncoder(out, resp.BulkList), nil
	}
	// len = 1 -> single element popped -> bulk string
	return resp.RESPEncoder(out[0], resp.Bulk), nil
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
		ch = make(chan string, 1)
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
			res = resp.RESPEncoder(out, resp.BulkList)
		} else {
			ch = make(chan string, 1)
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
		res = resp.RESPEncoder(out, resp.BulkList)

	// after acquiring lock if channel found and popped -> return -1
	// else value has been passed into channel -> process and return value
	case <-expch:
		kmu.Lock()
		defer kmu.Unlock()

		isExists := findAndPopCh(ch, key, com.Base)
		// channel popped from the list, no value pushed
		if isExists {
			res = resp.RESPEncoder(nil, resp.NullBulkList)
		} else {
			// false -> chan was filled just before timeout -> process and return happy path
			pop := <-ch
			out = append(out, pop)
			res = resp.RESPEncoder(out, resp.BulkList)
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

	return resp.RESPEncoder(id, resp.Bulk), nil
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

	return resp.RESPEncoder(entries, resp.Array), nil
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
			if ids[i] == "$" {
				// any id after 0-0 is eligible
				// stream does not currently exist
				ids[i] = "0-0"
			}
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
			return resp.RESPEncoder(nil, resp.NullBulkList), nil
		}
		return resp.RESPEncoder(res, resp.Array), nil
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

			return resp.RESPEncoder(res, resp.Array), nil

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
					return resp.RESPEncoder(nil, resp.NullBulkList), nil
				}
				return resp.RESPEncoder(res, resp.Array), nil

			default: // null ch
				return resp.RESPEncoder(nil, resp.NullBulkList), nil
			}
		}
	}
}

func (com *Com) incr() ([]byte, error) {
	key := com.Args["key"][0]

	kmu.Lock()
	defer kmu.Unlock()

	e, ok := keyspace[key]
	if !ok {
		val := "1"
		keyspace[key] = NewStructure(TypeVar, &val)
		return resp.RESPEncoder(1, resp.Int), nil
	}

	v, ok := e.Data.(*string)
	if !ok {
		return nil, errors.New("Operation against a key holding the wrong kind of value")
	}

	val, err := strconv.ParseInt(*v, 10, 64)
	if err != nil {
		return nil, errors.New("value is not an integer or out of range")
	}

	if val == math.MaxInt64 {
		return nil, errors.New("increment or decrement would overflow")
	}

	val++
	*v = strconv.FormatInt(val, 10)
	return resp.RESPEncoder(int(val), resp.Int), nil
}

// isTxn is a per connection field
func (com *Com) multi(state *State) ([]byte, error) {
	if state.isTxn {
		return nil, errors.New("MULTI calls can not be nested")
	}
	state.isTxn = true
	return resp.RESPEncoder("OK", resp.Simple), nil
}

// txn endpoint
func (com *Com) exec(server *server.Server, state *State) ([]byte, error) {
	if !state.isTxn {
		return nil, errors.New("EXEC without MULTI")
	}

	kmu.Lock()
	// lock to access txn state
	// a second connection could be accessing txn
	// at the same time, set dirty
	// end txn and cleanup
	if state.txn.dirty {
		HandleTxnEnd(state)
		kmu.Unlock()
		return resp.RESPEncoder(nil, resp.NullBulkList), nil
	}
	kmu.Unlock()

	var res []string = []string{}

	for _, com := range state.queue {
		out, err := com.HandleCom(server, state)
		if err != nil {
			e := resp.RESPEncoder(err.Error(), resp.SimpleErr)
			res = append(res, string(e))
		} else {
			res = append(res, string(out))
		}
	}

	kmu.Lock()
	HandleTxnEnd(state)
	kmu.Unlock()

	return resp.RESPEncoder(res, resp.Array), nil
}

func (com *Com) discard(state *State) ([]byte, error) {
	if !state.isTxn {
		return nil, errors.New("DISCARD without MULTI")
	}

	HandleTxnEnd(state)

	return resp.RESPEncoder("OK", resp.Simple), nil
}

func (com *Com) watch(state *State) ([]byte, error) {
	if state.isTxn {
		return nil, errors.New("WATCH inside MULTI is not allowed")
	}

	kmu.Lock()
	defer kmu.Unlock()

	for _, key := range com.Args["keys"] {
		state.txn.watched = append(state.txn.watched, key)
		watchers[key] = append(watchers[key], state.txn)
	}

	return resp.RESPEncoder("OK", resp.Simple), nil
}

func (com *Com) unwatch(state *State) ([]byte, error) {
	kmu.Lock()
	defer kmu.Unlock()

	CleanupWatcher(state.txn, state.txn.watched)
	state.txn.watched = []string{}
	state.txn.dirty = false

	return resp.RESPEncoder("OK", resp.Simple), nil
}

func (com *Com) info(srv *server.Server) ([]byte, error) {
	var s string
	m, ok := srv.IsMaster()
	if ok {
		s += "role:" + m.Role().String() + "\n"
		s += "master_replid:" + m.Id() + "\n"
		s += fmt.Sprintf("master_repl_offset:%d", m.Offset())
	} else {
		s += "role:slave"
	}
	return resp.RESPEncoder(s, resp.Bulk), nil
}
