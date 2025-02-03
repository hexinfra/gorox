package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	serverAddr, err := net.ResolveUDPAddr("udp", "192.168.1.2:9889")
	must(err)

	clientConn, err := net.DialUDP("udp", nil, serverAddr)
	must(err)
	defer clientConn.Close()

	p := make([]byte, 1200)

	beginTime := time.Now()
	_, err = clientConn.Write([]byte("hello, world"))
	must(err)

	n, err := clientConn.Read(p)
	must(err)

	fmt.Printf("%s, %s\n", p[:n], time.Since(beginTime).String())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
