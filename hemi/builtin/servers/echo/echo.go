// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
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
	RegisterServer("echoServer", func(compName string, stage *Stage) Server {
		s := new(echoServer)
		s.onCreate(compName, stage)
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

func (s *echoServer) onCreate(compName string, stage *Stage) {
	s.Server_.OnCreate(compName, stage)
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
		gate.onNew(s, id)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		go gate.Serve()
	}
	s.WaitGates()
	if DebugLevel() >= 2 {
		Printf("echoServer=%s done\n", s.CompName())
	}
	s.Stage().DecServer()
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
	Gate_[*echoServer]
	// States
	listener *net.TCPListener
}

func (g *echoGate) onNew(server *echoServer, id int32) {
	g.Gate_.OnNew(server, id)
}

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

func (g *echoGate) Serve() { // runner
	connID := int64(1)
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
		echoConn := getEchoConn(connID, g, tcpConn)
		go g.Server().serveConn(echoConn) // echoConn is put to pool in serve()
		connID++
	}
	g.WaitConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("echoGate=%d done\n", g.ID())
	}
	g.Server().DecGate()
}

func (g *echoGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.DecConn()
}

// echoConn
type echoConn struct {
	// Conn states (stocks)
	buffer [8152]byte
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64 // the conn id
	gate    *echoGate
	tcpConn *net.TCPConn
	// Conn states (zeros)
}

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

func (c *echoConn) closeConn() {
	c.tcpConn.Close()
	c.gate.DecConn()
}
