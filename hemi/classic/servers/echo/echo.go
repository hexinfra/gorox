// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Echo server. See RFC 862.

package echo

import (
	"context"
	"io"
	"net"
	"sync"
	"syscall"

	"github.com/hexinfra/gorox/hemi/library/system"

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
	// Parent
	Server_[*echoGate]
	// Assocs
	// States
}

func (s *echoServer) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)
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
		gate.init(id, s)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub() // gate
		go gate.serve()
	}
	s.WaitSubs() // gates
	if DebugLevel() >= 2 {
		Printf("echoServer=%s done\n", s.Name())
	}
	s.Stage().DecSub() // server
}

func (s *echoServer) serveConn(conn *echoConn) { // runner
	defer putEchoConn(conn)
	// TODO: deadline?
	io.CopyBuffer(conn.tcpConn, conn.tcpConn, conn.buffer[:])
	conn.closeConn()
}

// echoGate
type echoGate struct {
	// Parent
	Gate_
	// Assocs
	server *echoServer
	// States
	listener *net.TCPListener
}

func (g *echoGate) init(id int32, server *echoServer) {
	g.Gate_.Init(id, server.MaxConnsPerGate())
	g.server = server
}

func (g *echoGate) Server() Server  { return g.server }
func (g *echoGate) Address() string { return g.server.Address() }
func (g *echoGate) IsUDS() bool     { return g.server.IsUDS() }
func (g *echoGate) IsTLS() bool     { return g.server.IsTLS() }

func (g *echoGate) Open() error {
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
func (g *echoGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serve()
}

func (g *echoGate) serve() { // runner
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncConn()
		if actives := g.IncActives(); g.ReachLimit(actives) {
			g.justClose(tcpConn)
			continue
		}
		echoConn := getEchoConn(connID, g, tcpConn)
		go g.server.serveConn(echoConn) // echoConn is put to pool in serve()
		connID++
	}
	g.WaitConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("echoGate=%d done\n", g.ID())
	}
	g.server.DecSub() // gate
}

func (g *echoGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.DecActives()
	g.DecConn()
}

// echoConn
type echoConn struct {
	// Conn states (stocks)
	buffer [8152]byte
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	gate    *echoGate
	tcpConn *net.TCPConn
	// Conn states (zeros)
}

// poolEchoConn
var poolEchoConn sync.Pool

func getEchoConn(id int64, gate *echoGate, tcpConn *net.TCPConn) *echoConn {
	var conn *echoConn
	if x := poolEchoConn.Get(); x == nil {
		conn = new(echoConn)
	} else {
		conn = x.(*echoConn)
	}
	conn.onGet(id, gate, tcpConn)
	return conn
}
func putEchoConn(conn *echoConn) {
	conn.onPut()
	poolEchoConn.Put(conn)
}

func (c *echoConn) onGet(id int64, gate *echoGate, tcpConn *net.TCPConn) {
	c.id = id
	c.gate = gate
	c.tcpConn = tcpConn
}
func (c *echoConn) onPut() {
	c.tcpConn = nil
	c.gate = nil
}

func (c *echoConn) IsUDS() bool { return c.gate.IsUDS() }
func (c *echoConn) IsTLS() bool { return c.gate.IsTLS() }

func (c *echoConn) closeConn() {
	c.tcpConn.Close()
	c.gate.DecActives()
	c.gate.DecConn()
}
