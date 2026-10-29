package main

import (
	"errors"
	"strconv"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
	"github.com/codecrafters-io/redis-starter-go/app/server"
)

func (com *Com) replconf(srv *server.Server) ([]byte, error) {
	_, ok := com.Extras["ack"]
	if ok {
		return com.ack(srv)
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

func (com *Com) ack(srv *server.Server) ([]byte, error) {
	r, ok := srv.IsReplica()
	if !ok {
		return nil, errors.New("Not a replica")
	}
	res := []string{"REPLCONF", "ACK", strconv.Itoa(r.MasterOffset)}
	return resp.RESPEncoder(res, resp.BulkList), nil
}

func (com *Com) wait(srv *server.Server) ([]byte, error) {
	m, ok := srv.IsMaster()
	if !ok {
		return nil, errors.New("Not the master node")
	}

	return resp.RESPEncoder(m.GetConnCount(), resp.Int), nil
}
