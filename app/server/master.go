package server

import (
	"net"
	"sync"
)

type MasterRepl struct {
	id              string
	replConnections []net.Conn
	offset          int
	Mu              *sync.Mutex
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

func (m *MasterRepl) incrOffset(v int) {
	m.offset += v
}

// handles command propogation, master -> replica
func (m *MasterRepl) Propogate(b []byte) error {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.incrOffset(len(b))
	for _, conn := range m.replConnections {
		_, err := conn.Write(b)
		if err != nil {
			return err
		}
	}
	return nil
}

// handles simple message broadcast without a lock
// lock handled by the caller
func (m *MasterRepl) Broadcast(b []byte) error {
	for _, conn := range m.replConnections {
		_, err := conn.Write(b)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *MasterRepl) GetConnCount() int {
	return len(m.replConnections)
}

func (m *MasterRepl) AddConn(conn net.Conn) {
	m.replConnections = append(m.replConnections, conn)
}
