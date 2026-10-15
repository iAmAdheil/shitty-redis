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
	Type DataType
	Data any
}

var keyspace = make(map[string]*Structure)
var blpopWaiters = make(map[string][]chan string)
var xreadWaiters = make(map[string][]chan string)
var kmu *sync.RWMutex = &sync.RWMutex{}

func NewStructure(t DataType, data any) *Structure {
	return &Structure{
		Type: t,
		Data: data,
	}
}

func WakeAllXREADListeners(key string) {
	for _, ch := range xreadWaiters[key] {
		ch <- key
	}
}
