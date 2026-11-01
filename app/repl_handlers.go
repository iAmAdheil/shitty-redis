package main

import (
	"errors"
	"strconv"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
	"github.com/codecrafters-io/redis-starter-go/app/server"
)

func (com *Com) getack(srv *server.Server) ([]byte, error) {
	r, ok := srv.IsReplica()
	if !ok {
		return nil, errors.New("Not a replica")
	}
	res := []string{"REPLCONF", "ACK", strconv.Itoa(r.Offset)}
	return resp.RESPEncoder(res, resp.BulkList), nil
}

func (com *Com) ack(srv *server.Server, offset int) ([]byte, error) {
	m, ok := srv.IsMaster()
	if !ok {
		return nil, errors.New("Not a replica")
	}

	if m.Ackch != nil {
		select {
		case m.Ackch <- offset:
		case <-m.Donech:
		}
	}

	return nil, nil
}

func (com *Com) replconf(srv *server.Server) ([]byte, error) {
	_, ok := com.Extras["getack"]
	if ok {
		return com.getack(srv)
	}
	_, ok = com.Extras["ack"]
	if ok {
		s := com.Extras["offset"][0]
		offset, err := strconv.Atoi(s)
		if err != nil {
			return nil, errors.New("Invalid offset")
		}

		return com.ack(srv, offset)
	}

	return resp.RESPEncoder("OK", resp.Simple), nil
}

func (com *Com) psync(srv *server.Server) ([]byte, error) {
	m, ok := srv.IsMaster()
	if !ok {
		return nil, errors.New("Not a master node")
	}

	// start lock here, and release lock once rdb is sent
	// prevent offset corruption for the replica -> missing propogations
	// wait until conn is added to replconnections
	m.Mu.Lock()
	r := "FULLRESYNC" + " " + m.Id() + " " + strconv.Itoa(m.Offset())
	return resp.RESPEncoder(r, resp.Simple), nil
}

func (com *Com) wait(srv *server.Server) ([]byte, error) {
	var count int
	m, ok := srv.IsMaster()
	if !ok {
		return nil, errors.New("Not the master node")
	}

	s := com.Args["target"][0]
	target, err := strconv.Atoi(s)
	if err != nil {
		return nil, errors.New("Invalid replica count")
	}

	s = com.Args["timeout"][0]
	timeout, err := strconv.ParseFloat(s, 10)
	if err != nil {
		return nil, errors.New("Invalid timeout value")
	}
	duration := time.Duration(timeout * float64(time.Millisecond))
	expch := time.After(duration)

	m.Mu.Lock()
	defer m.Mu.Unlock()

	// handle cases -> 0 replicas, 0 commands propogated yet
	if m.GetConnCount() == 0 {
		return resp.RESPEncoder(0, resp.Int), nil
	}
	// does not include offset from GETACK broadcast
	moffset := m.Offset()
	if moffset == 0 {
		return resp.RESPEncoder(m.GetConnCount(), resp.Int), nil
	}

	m.Ackch = make(chan int)
	m.Donech = make(chan any)

	b := resp.RESPEncoder([]string{"REPLCONF", "GETACK", "*"}, resp.BulkList)
	if err := m.Broadcast(b); err != nil {
		return nil, err
	}

	for count < target {
		select {
		case roffset := <-m.Ackch:
			if roffset == moffset {
				count++
			}

		case <-expch:
			close(m.Donech)
			close(m.Ackch)
			m.Ackch = nil
			m.Donech = nil
			return resp.RESPEncoder(count, resp.Int), nil
		}
	}

	close(m.Donech)
	close(m.Ackch)
	m.Ackch = nil
	m.Donech = nil

	return resp.RESPEncoder(count, resp.Int), nil
}
