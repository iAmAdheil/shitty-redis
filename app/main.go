package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	l, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
		os.Exit(1)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}

		go HandleConn(conn)
	}
}

func HandleConn(conn net.Conn) {
	for {
		in := make([]byte, 1000)

		n, err := conn.Read(in)
		if err != nil {
			fmt.Printf("Error reading from connection: %s\n", err.Error())
			break
		}

		var out []byte

		base, args, err := RESPDecoder(in[:n])
		if err != nil {
			// do something
		}

		com, err := GetCom(base, args)
		if err != nil {
			out = RESPEncoder(err.Error(), SimpleErr)
		} else {
			out, err = com.HandleCom()
			if err != nil {
				out = RESPEncoder(err.Error(), SimpleErr)
			}
		}

		// log sent over request
		// for _, v := range parts {
		// 	fmt.Printf("%q\n", v)
		// }

		if _, err := conn.Write(out); err != nil {
			fmt.Printf("Error writing into connection: %s\n", err.Error())
		}
	}
}
