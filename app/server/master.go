package server

import (
	"net"
	"sync"
)

type MasterRepl struct {
	id              string
	replConnections []net.Conn
	offset          int
	mu              *sync.Mutex
}

func NewMaster() *MasterRepl {
	return &MasterRepl{
		id:     "8371b4fb1155b71f4a04d3e1bc3e18c4a990aeeb",
		offset: 0,
	}
}

func (m *MasterRepl) Role() Role {
	return Master
}

func (m *MasterRepl) Id() string {
	return m.id
}

func (m *MasterRepl) Offset() int {
	return m.offset
}

func (m *MasterRepl) IncrOffset(v int) {
	m.mu.Lock()
	m.offset += v
	m.mu.Unlock()
}

func (m *MasterRepl) Broadcast(b []byte) error {
	return nil
}

func (m *MasterRepl) GetConnCount() int {
	return len(m.replConnections)
}

func (m *MasterRepl) AddConn(conn net.Conn) {
	m.mu.Lock()
	m.replConnections = append(m.replConnections, conn)
	m.mu.Unlock()
}
