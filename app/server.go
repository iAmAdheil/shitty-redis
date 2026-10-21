package main

import (
	"flag"
	"fmt"
)

type Role int

const (
	Master Role = iota
	Replica
)

var roleName = map[Role]string{
	Master:  "master",
	Replica: "slave",
}

func (r Role) String() string {
	return roleName[r]
}

type Config struct {
	Port string
	Role Role
}

func ExtractConfig() *Config {
	port := flag.String("port", "6379", "port for redis server")
	replicaof := flag.String("replicaof", "", "server's role, as a replica or master")
	flag.Parse()

	var role Role = Master
	if len(*replicaof) > 0 {
		role = Replica
	}

	return &Config{
		Port: *port,
		Role: role,
	}
}

type Server struct {
	Address string
	Role    Role
}

func NewServer(c *Config) *Server {
	address := fmt.Sprintf("0.0.0.0:%s", c.Port)

	return &Server{
		Address: address,
		Role:    c.Role,
	}
}
