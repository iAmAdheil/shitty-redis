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

func HandleTxnEnd(state *State) {
	CleanupWatcher(state.txn, state.txn.watched)
	state.txn.watched = []string{}
	state.txn.dirty = false
	state.isTxn = false
	state.queue = []*Com{}
}

var watchers map[string][]*Txn = make(map[string][]*Txn)

func SetWatchingTxnsDirty(key string) {
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

// clean up watching txns from a given set of keys
func CleanupWatcher(t *Txn, keys []string) {
	for _, k := range keys {
		txns, ok := watchers[k]
		if !ok {
			continue
		}

		var clone []*Txn
		for _, txn := range txns {
			if txn != t {
				clone = append(clone, txn)
			}
		}

		watchers[k] = clone
	}
}
