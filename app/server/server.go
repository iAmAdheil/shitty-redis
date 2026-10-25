package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
)

type Replication interface {
	Role() Role
}

type Server struct {
	Port    string
	Address string
	Repl    Replication
}

func New(c *Config) *Server {
	address := fmt.Sprintf("0.0.0.0:%s", c.Port)

	var repl Replication
	if c.Role == Master {
		repl = NewMaster()
	} else if c.Role == Replica {
		repl = NewReplica(c)
	}

	return &Server{
		Port:    c.Port,
		Address: address,
		Repl:    repl,
	}
}

func (srv *Server) Start(cb func(*Server, net.Conn)) {
	_, ok := srv.IsReplica()
	if ok {
		err := srv.Handshake()
		if err != nil {
			fmt.Printf("Handshake Err: %s\n", err.Error())
		}
	}

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

func (srv *Server) Handshake() error {
	r, ok := srv.IsReplica()
	if r == nil || !ok {
		return errors.New("Not a replica")
	}

	addr := r.MasterHost + ":" + r.MasterPort
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	req := resp.RESPEncoder([]string{"PING"}, resp.BulkList)
	n, err := conn.Write(req)
	if err != nil {
		return err
	}

	fmt.Printf("%d bytes written to the master node, waiting for a response\n", n)

	res := make([]byte, 1024)
	n, err = conn.Read(res)
	if err != nil {
		return err
	}

	fmt.Printf("%d bytes received from the master node\n", n)

	r1 := resp.RESPEncoder([]string{"REPLCONF", "listening-port", srv.Port}, resp.BulkList)
	r2 := resp.RESPEncoder([]string{"REPLCONF", "capa", "psync2"}, resp.BulkList)

	_, err = conn.Write(r1)
	if err != nil {
		return err
	}
	fmt.Println("REPLCONF part 1 sent to the master node")

	r1res := make([]byte, 1024)
	n, err = conn.Read(r1res)
	if err != nil {
		return err
	}
	fmt.Println("REPLCONF part 1 response:", string(r1res))

	_, err = conn.Write(r2)
	if err != nil {
		return err
	}
	fmt.Println("REPLCONF part 2 sent to the master node")

	r2res := make([]byte, 1024)
	n, err = conn.Read(r2res)
	if err != nil {
		return err
	}
	fmt.Println("REPLCONF part 2 response:", string(r2res))

	sync := resp.RESPEncoder([]string{"PSYNC", "?", "-1"}, resp.BulkList)
	n, err = conn.Write(sync)
	if err != nil {
		return err
	}
	fmt.Println("PSYNC sent to the master node")

	syncres := make([]byte, 1024)
	n, err = conn.Read(syncres)
	if err != nil {
		return err
	}

	return nil
}
