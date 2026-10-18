package main

type State struct {
	isTxn bool
	queue []*Com
	txn   *Txn
}

type Txn struct {
	watched []string
	dirty   bool
}

var watchers map[string][]*Txn = make(map[string][]*Txn)

func NewState() *State {
	return &State{
		isTxn: false,
		queue: []*Com{},
		txn: &Txn{
			watched: []string{},
			dirty:   false,
		},
	}
}

func UpdateWatchers(key string) {
	txns, ok := watchers[key]
	if !ok {
		return
	}

	for _, txn := range txns {
		if txn != nil {
			txn.dirty = true
		}
	}
}
