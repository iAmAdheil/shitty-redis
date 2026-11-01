package server

type ReplicaRepl struct {
	MasterHost string
	MasterPort string
	MasterId   string
	Offset     int
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
