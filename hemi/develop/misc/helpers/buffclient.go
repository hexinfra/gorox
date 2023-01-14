package main

import (
	"flag"
	"fmt"
	"net"
)

var size int

func main() {
	flag.IntVar(&size, "size", 16384, "")
	flag.Parse()

	conn, err := net.Dial("tcp", "192.168.1.8:8888")
	must(err)
	defer conn.Close()

	p := make([]byte, size)
	got := 0
	all := 1 << 30 // 1GiB
	for got < all {
		n, err := conn.Write(p)
		if err != nil || n != len(p) {
			panic("write")
		}
		got += n
	}
	fmt.Println(got)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
