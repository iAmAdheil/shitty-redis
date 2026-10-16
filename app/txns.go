package main

type State struct {
	isMulti bool
	queue   []*Com
}

func NewState() *State {
	return &State{
		isMulti: false,
		queue:   []*Com{},
	}
}
