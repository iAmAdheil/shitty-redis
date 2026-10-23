package server

type MasterRepl struct {
	id     string
	offset int
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
