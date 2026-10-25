package main

import "github.com/codecrafters-io/redis-starter-go/app/resp"

func (com *Com) replconf() ([]byte, error) {
	return resp.RESPEncoder("OK", resp.Simple), nil
}
