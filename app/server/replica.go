package server

type ReplicaRepl struct {
	MasterHost   string
	MasterPort   string
	MasterId     string
	MasterOffset int
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
