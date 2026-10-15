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

// each structure has its own conditions for deletion
// checked by the caller, before func call
// handled by the caller, under its own lock phase
func DeleteStructure(key string) {
	delete(keyspace, key)
}
