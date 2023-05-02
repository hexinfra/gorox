// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1.1 implementation.

package main

import (
	"errors"
	"fmt"
	"net"
)

func http1() {
	serverAddr, err := net.ResolveTCPAddr("tcp", U.Host)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	request := []byte(fmt.Sprintf("%s %s HTTP/1.1\r\nHost: %s\r\n\r\n", M, U.Path, U.Host))
	for i := 0; i < C; i++ {
		benchmark.Add(1)
		client := newHTTP1Client(serverAddr, request)
		clients = append(clients, client)
	}
	for _, client := range clients {
		go client.bench()
	}
	benchmark.Wait()
}
func http1s() {
	fmt.Println("http1s")
}

type http1Client struct {
	serverAddr *net.TCPAddr
	request    []byte
	left       int
	statusGood int // 2xx, 3xx, 4xx
	statusBad  int // 5xx
	buffer     []byte
}

func newHTTP1Client(serverAddr *net.TCPAddr, request []byte) *http1Client {
	c := new(http1Client)
	c.serverAddr = serverAddr
	c.request = request
	c.left = R
	c.buffer = make([]byte, 16384)
	return c
}

func (c *http1Client) bench() {
	defer benchmark.Done()
	for c.left > 0 {
		serverConn, err := net.DialTCP("tcp", nil, c.serverAddr)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		for c.left > 0 {
			if _, err := serverConn.Write(c.request); err != nil {
				fmt.Println(err.Error())
				return
			}
			closed, err := c.recvResponse(serverConn)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			c.left--
			if closed {
				serverConn.Close()
				break
			}
		}
	}
}
func (c *http1Client) recvResponse(serverConn *net.TCPConn) (bool, error) {
	return false, errors.New("todo")
}
