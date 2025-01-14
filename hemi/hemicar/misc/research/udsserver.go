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
	for i := 0; i < 10000; i++ {
		c, e := l.Accept()
		must(e)
		go serve(c)
	}
	println(time.Now().Sub(begin).String())
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
