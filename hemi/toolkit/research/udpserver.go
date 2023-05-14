package main

import (
	"fmt"
	"net"
)

func main() {
	serverAddr, err := net.ResolveUDPAddr("udp", ":9889")
	must(err)

	serverGate, err := net.ListenUDP("udp", serverAddr)
	must(err)
	defer serverGate.Close()

	p := make([]byte, 1200)
	n, clientAddr, err := serverGate.ReadFromUDP(p)
	must(err)
	if false {
		fmt.Printf("%v %s %v\n", n, p[:n], clientAddr)
	}

	n, err = serverGate.WriteToUDP([]byte("xxxxxxx"), clientAddr)
	must(err)

	fmt.Printf("%v\n", n)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
