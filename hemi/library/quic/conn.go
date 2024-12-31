// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIC connection.

package quic

import (
	"net"
	"syscall"
	"time"
)

// Conn
type Conn struct {
	pktConn net.PacketConn
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
func (c *Conn) AcceptStream() (*Stream, error) {
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
