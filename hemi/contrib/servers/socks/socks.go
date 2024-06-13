// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// SOCKS proxy server.

// Don't confuse SOCKS with sock. We use "sock" as an abbreviation of "websocket".

package socks

import (
	"context"
	"net"
	"sync"
	"syscall"

	"github.com/hexinfra/gorox/hemi/common/system"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterServer("socksServer", func(name string, stage *Stage) Server {
		s := new(socksServer)
		s.onCreate(name, stage)
		return s
	})
}

// socksServer
type socksServer struct {
	// Parent
	Server_[*socksGate]
	// Assocs
	// States
}

func (s *socksServer) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)
}
func (s *socksServer) OnShutdown() {
	s.Server_.OnShutdown()
}

func (s *socksServer) OnConfigure() {
	s.Server_.OnConfigure()
}
func (s *socksServer) OnPrepare() {
	s.Server_.OnPrepare()
}

func (s *socksServer) Serve() { // runner
	for id := int32(0); id < s.NumGates(); id++ {
		gate := new(socksGate)
		gate.init(id, s)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub()
		go gate.serve()
	}
	s.WaitSubs() // gates
	if DebugLevel() >= 2 {
		Printf("socksServer=%s done\n", s.Name())
	}
	s.Stage().DecSub()
}

// socksGate
type socksGate struct {
	// Parent
	Gate_
	// Assocs
	// States
	listener *net.TCPListener
}

func (g *socksGate) init(id int32, server *socksServer) {
	g.Gate_.Init(id, server)
}

func (g *socksGate) Open() error {
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
func (g *socksGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serve()
}

func (g *socksGate) serve() { // runner
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
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			socksConn := getSocksConn(connID, g, tcpConn)
			go socksConn.serve() // socksConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("socksGate=%d done\n", g.ID())
	}
	g.Server().DecSub()
}

func (g *socksGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.OnConnClosed()
}

// poolSocksConn
var poolSocksConn sync.Pool

func getSocksConn(id int64, gate *socksGate, tcpConn *net.TCPConn) *socksConn {
	var conn *socksConn
	if x := poolSocksConn.Get(); x == nil {
		conn = new(socksConn)
	} else {
		conn = x.(*socksConn)
	}
	conn.onGet(id, gate, tcpConn)
	return conn
}
func putSocksConn(conn *socksConn) {
	conn.onPut()
	poolSocksConn.Put(conn)
}

// socksConn
type socksConn struct {
	// Parent
	ServerConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	tcpConn *net.TCPConn
	// Conn states (zeros)
}

func (c *socksConn) onGet(id int64, gate *socksGate, tcpConn *net.TCPConn) {
	c.ServerConn_.OnGet(id, gate)
	c.tcpConn = tcpConn
}
func (c *socksConn) onPut() {
	c.tcpConn = nil
	c.ServerConn_.OnPut()
}

func (c *socksConn) serve() { // runner
	defer putSocksConn(c)

	c.tcpConn.Write([]byte("not implemented yet"))
	c.closeConn()
}

func (c *socksConn) closeConn() {
	c.tcpConn.Close()
	c.Gate().OnConnClosed()
}
