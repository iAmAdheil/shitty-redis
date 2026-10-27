package server

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
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

func (srv *Server) Start(clientCb func(*Server, net.Conn, *bufio.Reader), replCb func(*Server, net.Conn, *bufio.Reader)) {
	_, ok := srv.IsReplica()
	if ok {
		c, r, err := srv.Handshake()
		if err != nil {
			fmt.Printf("Handshake Err: %s\n", err.Error())
		}
		go replCb(srv, c, r)
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

		r := bufio.NewReader(conn)

		go clientCb(srv, conn, r)
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

func (srv *Server) Handshake() (net.Conn, *bufio.Reader, error) {
	r, ok := srv.IsReplica()
	if r == nil || !ok {
		return nil, nil, errors.New("Not a replica")
	}

	addr := r.MasterHost + ":" + r.MasterPort
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, nil, err
	}

	reader := bufio.NewReader(conn)
	req := resp.RESPEncoder([]string{"PING"}, resp.BulkList)
	n, err := conn.Write(req)
	if err != nil {
		return nil, nil, err
	}

	fmt.Printf("%d bytes written to the master node, waiting for a response\n", n)

	res := make([]byte, 1024)
	n, err = reader.Read(res)
	if err != nil {
		return nil, nil, err
	}

	fmt.Printf("%d bytes received from the master node\n", n)

	r1 := resp.RESPEncoder([]string{"REPLCONF", "listening-port", srv.Port}, resp.BulkList)
	r2 := resp.RESPEncoder([]string{"REPLCONF", "capa", "psync2"}, resp.BulkList)

	_, err = conn.Write(r1)
	if err != nil {
		return nil, nil, err
	}
	fmt.Println("REPLCONF part 1 sent to the master node")

	r1res := make([]byte, 1024)
	n, err = reader.Read(r1res)
	if err != nil {
		return nil, nil, err
	}
	fmt.Println("REPLCONF part 1 response:", string(r1res))

	_, err = conn.Write(r2)
	if err != nil {
		return nil, nil, err
	}
	fmt.Println("REPLCONF part 2 sent to the master node")

	r2res := make([]byte, 1024)
	n, err = reader.Read(r2res)
	if err != nil {
		return nil, nil, err
	}
	fmt.Println("REPLCONF part 2 response:", string(r2res))

	sync := resp.RESPEncoder([]string{"PSYNC", "?", "-1"}, resp.BulkList)
	n, err = conn.Write(sync)
	if err != nil {
		return nil, nil, err
	}
	fmt.Println("PSYNC sent to the master node")

	syncres, err := reader.ReadString('\n')
	if err != nil {
		return nil, nil, err
	}
	fmt.Printf("PSYNC response: %s\n", syncres)
	// extract replica offset from the response
	syncres = string(bytes.TrimSpace([]byte(syncres)))
	os := strings.Split(syncres, " ")[2]
	offset, err := strconv.Atoi(os)
	if err != nil {
		return nil, nil, err
	}
	// update replica -> starting offset
	r.MasterOffset = offset

	// wait for rdb after psync
	bcs, err := reader.ReadString('\n')
	if err != nil {
		return nil, nil, err
	}
	bcs = string(bytes.TrimSpace([]byte(bcs)))
	bc, err := strconv.Atoi(bcs[1:])
	if err != nil {
		return nil, nil, err
	}
	rdb := make([]byte, bc)
	_, err = reader.Read(rdb)
	if err != nil {
		return nil, nil, err
	}

	return conn, reader, nil
}
