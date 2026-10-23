package server

import (
	"fmt"
	"net"
	"os"
)

type Replication interface {
	Role() Role
}

type Server struct {
	Address string
	Repl    Replication
}

func New(c *Config) *Server {
	address := fmt.Sprintf("0.0.0.0:%s", c.Port)

	var repl Replication
	if c.Role == Master {
		repl = NewMaster()
	} else if c.Role == Replica {
		repl = NewReplica()
	}

	return &Server{
		Address: address,
		Repl:    repl,
	}
}

func (srv *Server) Start(cb func(*Server, net.Conn)) {
	l, err := net.Listen("tcp", srv.Address)
	if err != nil {
		fmt.Println("Error:", err.Error())
		os.Exit(1)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}

		go cb(srv, conn)
	}
}

func (srv *Server) IsMaster() (*MasterRepl, bool) {
	m, ok := srv.Repl.(*MasterRepl)
	if !ok {
		return nil, false
	}
	return m, true
}

func (srv *Server) IsReplica() (*ReplicaRepl, bool) {
	m, ok := srv.Repl.(*ReplicaRepl)
	if !ok {
		return nil, false
	}
	return m, true
}
