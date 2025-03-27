package main

import (
	"net"
	"time"
)

func main() {
	//l, e := net.Listen("tcp", ":9889")
	l, e := net.Listen("unix", "a.sock")
	must(e)
	begin := time.Now()
	for _ = range 10000 {
		c, e := l.Accept()
		must(e)
		go serve(c)
	}
	println(time.Since(begin).String())
	l.Close()
}

func serve(c net.Conn) {
	c.Close()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
