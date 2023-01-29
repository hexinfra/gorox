// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC conn.

package quix

import (
	"net"
	"syscall"
	"time"
)

// Conn
type Conn struct {
	udpConn *net.UDPConn
	rawConn syscall.RawConn
}

func Dial(address string) (*Conn, error) {
	return nil, nil
}
func DialTimeout(address string, timeout time.Duration) (*Conn, error) {
	return nil, nil
}

func (c *Conn) CreateStream() (*Stream, error) {
	stream := new(Stream)
	stream.conn = c
	return stream, nil
}
func (c *Conn) CreateOneway() (*Oneway, error) {
	stream := new(Oneway)
	stream.conn = c
	return stream, nil
}
func (c *Conn) AcceptStream() (*Stream, error) {
	return nil, nil
}
func (c *Conn) AcceptOneway() (*Oneway, error) {
	return nil, nil
}

func (c *Conn) SendMsg(p []byte) error { // see RFC 9221
	return nil
}
func (c *Conn) RecvMsg(p []byte) error { // see RFC 9221
	return nil
}

func (c *Conn) Close() error {
	return nil
}
