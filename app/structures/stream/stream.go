package stream

type Id struct {
	ms  uint64
	seq uint64
}

type Stream struct {
	// Rax               map[Id]*listpack.Listpack
	Length            int
	LastId            Id
	MaxDeletedEntryId Id
	EntriesAdded      int
	// cgroups
}
