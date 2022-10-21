package main

import (
	"fmt"
	"net"
)

func main() {
	serverAddr, err := net.ResolveUDPAddr("udp", "192.168.1.2:9889")
	must(err)
	clientConn, err := net.DialUDP("udp", nil, serverAddr)
	must(err)
	defer clientConn.Close()

	n, err := clientConn.Write([]byte("hello, world"))
	must(err)
	fmt.Printf("%v\n", n)

	p := make([]byte, 1200)
	n, err = clientConn.Read(p)
	must(err)
	fmt.Printf("%s\n", p[:n])
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
