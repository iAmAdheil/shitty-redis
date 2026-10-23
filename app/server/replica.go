package server

type ReplicaRepl struct {
	// empty
}

func NewReplica() *ReplicaRepl {
	return &ReplicaRepl{}
}

func (r *ReplicaRepl) Role() Role {
	return Replica
}
