package main

import (
	"fmt"
	"net"
)

func main() {
	gate, err := net.Listen("tcp", ":8888")
	must(err)
	defer gate.Close()

	conn, err := gate.Accept()
	must(err)
	defer conn.Close()

	p := make([]byte, 1048576) // 1MiB
	a := 0
	for {
		n, err := conn.Read(p)
		a += n
		if err != nil {
			break
		}
	}
	fmt.Println(a)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
