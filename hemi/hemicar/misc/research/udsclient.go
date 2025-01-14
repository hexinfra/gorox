package main

import (
	"net"
)

func main() {
	for i := 0; i < 10000; i++ {
		//c, e := net.Dial("tcp", "127.0.0.1:9889")
		c, e := net.Dial("unix", "a.sock")
		must(e)
		c.Close()
	}
	println("done")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
