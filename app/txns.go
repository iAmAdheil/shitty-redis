package main

type State struct {
	isTxn bool
	queue []*Com
}

func NewState() *State {
	return &State{
		isTxn: false,
		queue: []*Com{},
	}
}
