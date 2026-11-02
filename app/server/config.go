package server

import (
	"errors"
	"flag"
	"strings"
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
	Port        string
	Role        Role
	MasterHost  string
	MasterPort  string
	RDBDir      string
	RDBFilename string
}

func ExtractConfig() *Config {
	port := flag.String("port", "6379", "port for redis server")
	replicaof := flag.String("replicaof", "", "server's role, as a replica or master")
	dir := flag.String("dir", "../tmp/redis-db", "path to the directory where the RDB file is stored")
	dbfilename := flag.String("dbfilename", "dump.rdb", "name of the RDB file")
	flag.Parse()

	cfg := &Config{
		Port:        *port,
		RDBDir:      *dir,
		RDBFilename: *dbfilename,
	}

	cfg.Role = Master
	if len(*replicaof) > 0 {
		cfg.Role = Replica
		cfg.ReplCfg(*replicaof)
	}

	return cfg
}

func (cfg *Config) ReplCfg(mc string) error {
	s := strings.Split(mc, " ")
	if len(s) <= 1 {
		return errors.New("Invalid master connection configuration")
	}

	h, p := s[0], s[1]
	cfg.MasterHost = h
	cfg.MasterPort = p

	return nil
}
