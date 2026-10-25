package main

import (
	"fmt"
	"net"

	rdb "github.com/codecrafters-io/redis-starter-go/app/RDB"
	"github.com/codecrafters-io/redis-starter-go/app/resp"
	"github.com/codecrafters-io/redis-starter-go/app/server"
)

func main() {
	cfg := server.ExtractConfig()
	srv := server.New(cfg)
	srv.Start(HandleConn)
}

func HandleConn(server *server.Server, conn net.Conn) {
	state := NewState()

	for {
		in := make([]byte, 1000)

		n, err := conn.Read(in)
		if err != nil {
			fmt.Printf("Error reading from connection: %s\n", err.Error())
			break
		}

		var out []byte

		base, args, err := resp.RESPDecoder(in[:n])
		if err != nil {
			// do something
		}

		com, err := GetCom(base, args)
		if err != nil {
			out = resp.RESPEncoder(err.Error(), resp.SimpleErr)
		} else {
			if state.isTxn && com.Base != "exec" && com.Base != "multi" && com.Base != "discard" && com.Base != "watch" && com.Base != "unwatch" {
				state.queue = append(state.queue, com)
				out = resp.RESPEncoder("QUEUED", resp.Simple)
			} else {
				out, err = com.HandleCom(server, state)
				if err != nil {
					out = resp.RESPEncoder(err.Error(), resp.SimpleErr)
				}
			}
		}

		// log sent over request
		// for _, v := range parts {
		// 	fmt.Printf("%q\n", v)
		// }

		if _, err := conn.Write(out); err != nil {
			fmt.Printf("Error writing into connection: %s\n", err.Error())
			continue
		}

		// propogation
		m, ok := server.IsMaster()
		if ok && (com.Base == "set") {
			for _, c := range m.ReplConnections {
				c.Write(resp.RESPEncoder(append([]string{base}, args...), resp.BulkList))
			}
		}

		m, ok = server.IsMaster()
		if ok && com.Base == "psync" {
			e := rdb.EmptyRDB()
			header := []byte(fmt.Sprintf("$%d\r\n", len(e)))

			if _, err := conn.Write(append(header, e...)); err != nil {
				fmt.Printf("RDB transfer (PSYNC) failure for replica address: %s\n", conn.RemoteAddr().String())
				continue
			}

			if ok {
				m.ReplConnections = append(m.ReplConnections, conn)
			}
		}
	}
}
