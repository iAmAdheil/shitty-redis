package main

import (
	"sync"
)

type DataType int

const (
	TypeVar  DataType = iota // store *string
	TypeList                 // store *list.List
	TypeStream
)

type Structure struct {
	Type      DataType
	Data      any
	Listeners []chan string // waiters blocked on this key (BLPOP, XREAD BLOCK)
}

var keyspace = make(map[string]*Structure)
var kmu *sync.RWMutex = &sync.RWMutex{}

func NewStructure(t DataType, data any) *Structure {
	return &Structure{
		Type: t,
		Data: data,
		// init empty
		Listeners: []chan string{},
	}
}
