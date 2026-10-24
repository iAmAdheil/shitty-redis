package server

import (
	"fmt"
	"net"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
)

type ReplicaRepl struct {
	MasterHost string
	MasterPort string
}

func NewReplica(c *Config) *ReplicaRepl {
	return &ReplicaRepl{
		MasterHost: c.MasterHost,
		MasterPort: c.MasterPort,
	}
}

func (r *ReplicaRepl) Role() Role {
	return Replica
}

func (r *ReplicaRepl) Handshake() error {
	addr := r.MasterHost + ":" + r.MasterPort

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	ping := "$4\r\nPING\r\n"
	req := resp.RESPEncoder([]string{ping}, resp.Array)
	n, err := conn.Write(req)
	if err != nil {
		return err
	}

	fmt.Printf("%d bytes written to the master node, waiting for a response\n", n)

	// res := make([]byte, 1024)
	// n, err = conn.Read(res)
	// if err != nil {
	// 	return err
	// }
	//
	// fmt.Printf("%d bytes received from the master node\n", n)

	return nil
}
