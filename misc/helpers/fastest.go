
// This is the fastest result that an http server written in Go (with package net) can achieve.

package main

import (
	"fmt"
	"io"
	"net"
	"os"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	go server(":9999", handleBlob) // wrk -d 8s -c 256 -t 16 http://192.168.1.10:9999/benchmark
	go server(":8888", handleFile) // wrk -d 8s -c 256 -t 16 http://192.168.1.10:8888/benchmark.html
	select {}
}

func server(addr string, handle func(net.Conn)) {
	gate, err := net.Listen("tcp", addr)
	must(err)
	defer gate.Close()
	for {
		conn, err := gate.Accept()
		if err != nil {
			fmt.Printf("gate.Accept()=%s\n", err.Error())
			return
		}
		go handle(conn)
	}
}

func handleBlob(conn net.Conn) { // 96% of nginx
	defer conn.Close()
	request := make([]byte, 52) // GET /benchmark HTTP/1.1\r\nHost: 192.168.1.10:9999\r\n\r\n
	response := []byte("HTTP/1.1 200 OK\r\ndate: Sat, 08 Oct 2022 12:07:52 GMT\r\ncontent-length: 13\r\ncontent-type: text/html; charset=utf-8\r\nconnection: keep-alive\r\nserver: gorox\r\n\r\nhello, world!")
	for {
		if _, err := io.ReadFull(conn, request); err != nil { // syscall 1: read()
			fmt.Printf("io.ReadFull()=%s\n", err.Error())
			return
		}
		if _, err := conn.Write(response); err != nil { // syscall 2: write()
			fmt.Printf("conn.Write()=%s\n", err.Error())
			return
		}
	}
}

func handleFile(conn net.Conn) { // 70% of nginx
	defer conn.Close()
	request := make([]byte, 57) // GET /benchmark.html HTTP/1.1\r\nHost: 192.168.1.10:8888\r\n\r\n
	head := []byte("HTTP/1.1 200 OK\r\ncontent-type: text/html\r\ndate: Sat, 08 Oct 2022 12:09:04 GMT\r\nlast-modified: Fri, 23 Sep 2022 18:21:12 GMT\r\netag: \"632df918-93\"\r\ncontent-length: 147\r\nconnection: keep-alive\r\naccept-ranges: bytes\r\nserver: gorox\r\n\r\n")
	content := make([]byte, 147)
	var response [2][]byte
	for {
		if _, err := io.ReadFull(conn, request); err != nil { // syscall 1: read()
			fmt.Printf("io.ReadFull(1)=%s\n", err.Error())
			return
		}
		file, err := os.Open("benchmark.html") // syscall 2: openat()
		if err != nil {
			fmt.Printf("os.Open()=%s\n", err.Error())
			return
		}
		stat, err := file.Stat() // syscall 3: newfstatat()
		if err != nil {
			fmt.Printf("file.Stat()=%s\n", err.Error())
			file.Close()
			return
		}
		_, err = io.ReadFull(file, content[0:stat.Size()]) // syscall 4: read()
		if err != nil {
			fmt.Printf("io.ReadFull(2)=%s\n", err.Error())
			return
		}
		response = [2][]byte{head, content}
		buffers := net.Buffers(response[:])
		if _, err := buffers.WriteTo(conn); err != nil { // syscall 5: writev()
			fmt.Printf("buffers.WriteTo()=%s\n", err.Error())
			return
		}
		/*
			if _, err := conn.Write(head); err != nil {
				fmt.Printf("conn.Write(1)=%s\n", err.Error())
				return
			}
			if _, err := conn.Write(content); err != nil {
				fmt.Printf("conn.Write(2)=%s\n", err.Error())
				return
			}
		*/
		file.Close() // syscall 6: close()
	}
}
