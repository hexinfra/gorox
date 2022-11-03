// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Echo server. See RFC 862.

package echo

import (
	"context"
	. "github.com/hexinfra/gorox/hemi/internal"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"net"
	"sync"
	"syscall"
)

func init() {
	RegisterServer("echoServer", func(name string, stage *Stage) Server {
		s := new(echoServer)
		s.init(name, stage)
		return s
	})
}

// echoServer
type echoServer struct {
	// Mixins
	Server_
	// Assocs
	// States
	gates []*echoGate
}

func (s *echoServer) init(name string, stage *Stage) {
	s.Server_.Init(name, stage)
}

func (s *echoServer) OnConfigure() {
	s.Server_.OnConfigure()
}
func (s *echoServer) OnPrepare() {
	s.Server_.OnPrepare()
}
func (s *echoServer) OnShutdown() {
	s.Server_.OnShutdown()
}

func (s *echoServer) Serve() {
	for id := int32(0); id < s.NumGates(); id++ {
		gate := new(echoGate)
		gate.init(s, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		s.gates = append(s.gates, gate)
		go gate.serve()
	}
	select {}
}

// echoGate
type echoGate struct {
	// Mixins
	Gate_
	// Assocs
	server *echoServer
	// States
	listener *net.TCPListener
}

func (g *echoGate) init(server *echoServer, id int32) {
	g.Gate_.Init(server.Stage(), id, server.Address(), server.MaxConnsPerGate())
	g.server = server
}

func (g *echoGate) open() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		return system.SetReusePort(rawConn)
	}
	listener, err := listenConfig.Listen(context.Background(), "tcp", g.Address())
	if err == nil {
		g.listener = listener.(*net.TCPListener)
	}
	return err
}

func (g *echoGate) serve() {
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
		if err != nil {
			continue
		}
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			echoConn := getEchoConn(connID, g.Stage(), g.server, g, tcpConn)
			go echoConn.serve() // echoConn is put to pool in serve()
			connID++
		}
	}
}

func (g *echoGate) onConnectionClosed() {
	g.DecConns()
}
func (g *echoGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}

// poolEchoConn
var poolEchoConn sync.Pool

func getEchoConn(id int64, stage *Stage, server *echoServer, gate *echoGate, netConn *net.TCPConn) *echoConn {
	var conn *echoConn
	if x := poolEchoConn.Get(); x == nil {
		conn = new(echoConn)
	} else {
		conn = x.(*echoConn)
	}
	conn.onGet(id, stage, server, gate, netConn)
	return conn
}
func putEchoConn(conn *echoConn) {
	conn.onPut()
	poolEchoConn.Put(conn)
}

// echoConn
type echoConn struct {
	// Conn states (buffers)
	buffer [8152]byte
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	stage   *Stage
	server  *echoServer
	gate    *echoGate
	netConn *net.TCPConn
	// Conn states (zeros)
}

func (c *echoConn) onGet(id int64, stage *Stage, server *echoServer, gate *echoGate, netConn *net.TCPConn) {
	c.id = id
	c.stage = stage
	c.server = server
	c.gate = gate
	c.netConn = netConn
}
func (c *echoConn) onPut() {
	c.stage = nil
	c.server = nil
	c.gate = nil
	c.netConn = nil
}

func (c *echoConn) serve() {
	defer putEchoConn(c)
	for {
		// TODO: set deadline?
		n, err := c.netConn.Read(c.buffer[:])
		if err != nil {
			break
		}
		if _, err := c.netConn.Write(c.buffer[:n]); err != nil {
			break
		}
	}
	c.closeConn()
}

func (c *echoConn) closeConn() {
	c.netConn.Close()
	c.gate.onConnectionClosed()
}
