// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Echo server. See RFC 862.

package echo

import (
	"context"
	"io"
	"net"
	"sync"
	"syscall"

	"github.com/hexinfra/gorox/hemi/common/system"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterServer("echoServer", func(name string, stage *Stage) Server {
		s := new(echoServer)
		s.onCreate(name, stage)
		return s
	})
}

// echoServer
type echoServer struct {
	// Mixins
	Server_[*echoGate]
	// Assocs
	// States
}

func (s *echoServer) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)
}
func (s *echoServer) OnShutdown() {
	s.ShutGates()
}

func (s *echoServer) OnConfigure() {
	s.Server_.OnConfigure()
}
func (s *echoServer) OnPrepare() {
	s.Server_.OnPrepare()
}

func (s *echoServer) Serve() { // runner
	for id := int32(0); id < s.NumGates(); id++ {
		gate := new(echoGate)
		gate.init(s, id)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AppendGate(gate)
		s.IncSub(1)
		go gate.serve()
	}
	s.WaitSubs() // gates
	if Debug() >= 2 {
		Printf("echoServer=%s done\n", s.Name())
	}
	s.Stage().SubDone()
}

// echoGate
type echoGate struct {
	// Mixins
	Gate_
	// Assocs
	server *echoServer
	// States
	gate *net.TCPListener
}

func (g *echoGate) init(server *echoServer, id int32) {
	g.Gate_.Init(server.Stage(), id, server.Address(), server.MaxConnsPerGate())
	g.server = server
}

func (g *echoGate) Open() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		return system.SetReusePort(rawConn)
	}
	gate, err := listenConfig.Listen(context.Background(), "tcp", g.Address())
	if err == nil {
		g.gate = gate.(*net.TCPListener)
	}
	return err
}
func (g *echoGate) Shut() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *echoGate) serve() { // runner
	connID := int64(0)
	for {
		tcpConn, err := g.gate.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub(1)
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			echoConn := getEchoConn(connID, g.Stage(), g.server, g, tcpConn)
			go echoConn.serve() // echoConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("echoGate=%d done\n", g.ID())
	}
	g.server.SubDone()
}

func (g *echoGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.OnConnClosed()
}

// poolEchoConn
var poolEchoConn sync.Pool

func getEchoConn(id int64, stage *Stage, server *echoServer, gate *echoGate, tcpConn *net.TCPConn) *echoConn {
	var conn *echoConn
	if x := poolEchoConn.Get(); x == nil {
		conn = new(echoConn)
	} else {
		conn = x.(*echoConn)
	}
	conn.onGet(id, stage, server, gate, tcpConn)
	return conn
}
func putEchoConn(conn *echoConn) {
	conn.onPut()
	poolEchoConn.Put(conn)
}

// echoConn
type echoConn struct {
	// Conn states (stocks)
	buffer [8152]byte
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	stage   *Stage
	server  *echoServer
	gate    *echoGate
	tcpConn *net.TCPConn
	// Conn states (zeros)
}

func (c *echoConn) onGet(id int64, stage *Stage, server *echoServer, gate *echoGate, tcpConn *net.TCPConn) {
	c.id = id
	c.stage = stage
	c.server = server
	c.gate = gate
	c.tcpConn = tcpConn
}
func (c *echoConn) onPut() {
	c.stage = nil
	c.server = nil
	c.gate = nil
	c.tcpConn = nil
}

func (c *echoConn) serve() { // runner
	defer putEchoConn(c)
	// TODO: deadline?
	io.CopyBuffer(c.tcpConn, c.tcpConn, c.buffer[:])
	c.closeConn()
}

func (c *echoConn) closeConn() {
	c.tcpConn.Close()
	c.gate.OnConnClosed()
}
