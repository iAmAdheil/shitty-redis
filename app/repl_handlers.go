package main

import (
	"errors"
	"strconv"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
	"github.com/codecrafters-io/redis-starter-go/app/server"
)

func (com *Com) replconf() ([]byte, error) {
	return resp.RESPEncoder("OK", resp.Simple), nil
}

func (com *Com) psync(srv *server.Server) ([]byte, error) {
	m, ok := srv.IsMaster()
	if !ok {
		return nil, errors.New("Not a master node")
	}

	r := "FULLRESYNC" + " " + m.Id() + " " + strconv.Itoa(m.Offset())
	return resp.RESPEncoder(r, resp.Simple), nil
}
