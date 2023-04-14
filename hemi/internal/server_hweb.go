// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB server implementation.

// HWEB doesn't support WebSocket.

package internal

import (
	"net"
	"sync"
	"syscall"
)

// hwebServer is the HWEB server.
type hwebServer struct {
	// Mixins
	webServer_
	// Assocs
	// States
}

func (s *hwebServer) OnShutdown() {
}

func (s *hwebServer) Serve() { // goroutine
}

// hwebGate is a gate of hwebServer.
type hwebGate struct {
	// Mixins
	webGate_
	// Assocs
	server *hwebServer
	// States
	gate *net.TCPListener
}

func (g *hwebGate) shutdown() error {
	return nil
}

func (g *hwebGate) serveTCP() { // goroutine
}
func (g *hwebGate) serveTLS() { // goroutine
}

// poolHWEBConn is the server-side HWEB connection pool.
var poolHWEBConn sync.Pool

func getHWEBConn(id int64, server *hwebServer, gate *hwebGate, netConn net.Conn, rawConn syscall.RawConn) webConn {
	var conn *hwebConn
	if x := poolHWEBConn.Get(); x == nil {
		conn = new(hwebConn)
	} else {
		conn = x.(*hwebConn)
	}
	conn.onGet(id, server, gate, netConn, rawConn)
	return conn
}
func putHWEBConn(conn *hwebConn) {
	conn.onPut()
	poolHWEBConn.Put(conn)
}

// hwebConn
type hwebConn struct {
	webConn_
}

func (c *hwebConn) onGet(id int64, server *hwebServer, gate *hwebGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.webConn_.onGet(id, server, gate)
}
func (c *hwebConn) onPut() {
	c.webConn_.onPut()
}

func (c *hwebConn) serve() { // goroutine
}

// poolHWEBStream is the server-side HWEB stream pool.
var poolHWEBStream sync.Pool

func getHWEBStream(conn *hwebConn, id uint32) *hwebStream {
	return nil
}
func putHWEBStream(stream *hwebStream) {
}

// hwebStream
type hwebStream struct {
	webStream_
}

func (s *hwebStream) execute() { // goroutine
}

// hwebRequest
type hwebRequest struct {
	webRequest_
}

// hwebResponse
type hwebResponse struct {
	webResponse_
}
