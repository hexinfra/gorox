// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC connection.

package quix

import (
	"net"
	"syscall"
	"time"
)

// Connection
type Connection struct {
	udpConn *net.UDPConn
	rawConn syscall.RawConn
}

func Dial(address string) (*Connection, error) {
	return nil, nil
}
func DialTimeout(address string, timeout time.Duration) (*Connection, error) {
	return nil, nil
}

func (c *Connection) CreateStream() (*Stream, error) {
	stream := new(Stream)
	stream.connection = c
	return stream, nil
}
func (c *Connection) CreateOneway() (*Oneway, error) {
	stream := new(Oneway)
	stream.connection = c
	return stream, nil
}
func (c *Connection) AcceptStream() (*Stream, error) {
	return nil, nil
}
func (c *Connection) AcceptOneway() (*Oneway, error) {
	return nil, nil
}

func (c *Connection) SendMsg(p []byte) error { // see RFC 9221
	return nil
}
func (c *Connection) RecvMsg(p []byte) error { // see RFC 9221
	return nil
}

func (c *Connection) Close() error {
	return nil
}
