// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is a hello server showing how to use Gorox to host a server.

package hello

import (
	"context"
	"net"
	"sync"
	"syscall"

	"github.com/hexinfra/gorox/hemi/common/system"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterServer("helloServer", func(name string, stage *Stage) Server {
		s := new(helloServer)
		s.onCreate(name, stage)
		return s
	})
}

// helloServer
type helloServer struct {
	// Parent
	Server_[*helloGate]
	// Assocs
	// States
}

func (s *helloServer) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)
}
func (s *helloServer) OnShutdown() {
	s.Server_.OnShutdown()
}

func (s *helloServer) OnConfigure() {
	s.Server_.OnConfigure()
}
func (s *helloServer) OnPrepare() {
	s.Server_.OnPrepare()
}

func (s *helloServer) Serve() { // runner
	for id := int32(0); id < s.NumGates(); id++ {
		gate := new(helloGate)
		gate.init(id, s)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		s.AddGate(gate)
		s.IncSub()
		go gate.serve()
	}
	s.WaitSubs() // gates
	if DbgLevel() >= 2 {
		Printf("helloServer=%s done\n", s.Name())
	}
	s.Stage().DecSub()
}

// helloGate
type helloGate struct {
	// Parent
	Gate_
	// Assocs
	// States
	listener *net.TCPListener
}

func (g *helloGate) init(id int32, server *helloServer) {
	g.Gate_.Init(id, server)
}

func (g *helloGate) Open() error {
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
func (g *helloGate) Shut() error {
	g.MarkShut()
	return g.listener.Close()
}

func (g *helloGate) serve() { // runner
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
			helloConn := getHelloConn(connID, g, tcpConn)
			go helloConn.serve() // helloConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DbgLevel() >= 2 {
		Printf("helloGate=%d done\n", g.ID())
	}
	g.Server().DecSub()
}

func (g *helloGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.OnConnClosed()
}

// poolHelloConn
var poolHelloConn sync.Pool

func getHelloConn(id int64, gate *helloGate, tcpConn *net.TCPConn) *helloConn {
	var conn *helloConn
	if x := poolHelloConn.Get(); x == nil {
		conn = new(helloConn)
	} else {
		conn = x.(*helloConn)
	}
	conn.onGet(id, gate, tcpConn)
	return conn
}
func putHelloConn(conn *helloConn) {
	conn.onPut()
	poolHelloConn.Put(conn)
}

// helloConn
type helloConn struct {
	ServerConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	tcpConn *net.TCPConn
	// Conn states (zeros)
}

func (c *helloConn) onGet(id int64, gate *helloGate, tcpConn *net.TCPConn) {
	c.ServerConn_.OnGet(id, gate)
	c.tcpConn = tcpConn
}
func (c *helloConn) onPut() {
	c.tcpConn = nil
	c.ServerConn_.OnPut()
}

func (c *helloConn) serve() { // runner
	defer putHelloConn(c)
	c.tcpConn.Write([]byte("hello, world!"))
	c.closeConn()
}

func (c *helloConn) closeConn() {
	c.tcpConn.Close()
	c.Gate().OnConnClosed()
}
