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
	// Mixins
	Server_
	// Assocs
	// States
	gates []*helloGate
}

func (s *helloServer) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)
}
func (s *helloServer) OnShutdown() {
	// We don't close(s.ShutChan) here.
	for _, gate := range s.gates {
		gate.shut()
	}
}

func (s *helloServer) OnConfigure() {
	s.Server_.OnConfigure()
}
func (s *helloServer) OnPrepare() {
	s.Server_.OnPrepare()
}

func (s *helloServer) Serve() { // goroutine
	for id := int32(0); id < s.NumGates(); id++ {
		gate := new(helloGate)
		gate.init(s, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		s.gates = append(s.gates, gate)
		s.IncSub(1)
		go gate.serve()
	}
	s.WaitSubs() // gates
	if Debug() >= 2 {
		Printf("helloServer=%s done\n", s.Name())
	}
	s.Stage().SubDone()
}

// helloGate
type helloGate struct {
	// Mixins
	Gate_
	// Assocs
	server *helloServer
	// States
	gate *net.TCPListener
}

func (g *helloGate) init(server *helloServer, id int32) {
	g.Gate_.Init(server.Stage(), id, server.Address(), server.MaxConnsPerGate())
	g.server = server
}

func (g *helloGate) open() error {
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
func (g *helloGate) shut() error {
	g.MarkShut()
	return g.gate.Close()
}

func (g *helloGate) serve() { // goroutine
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
			helloConn := getHelloConn(connID, g.Stage(), g.server, g, tcpConn)
			go helloConn.serve() // helloConn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if Debug() >= 2 {
		Printf("helloGate=%d done\n", g.ID())
	}
	g.server.SubDone()
}

func (g *helloGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnClosed()
}
func (g *helloGate) onConnClosed() {
	g.DecConns()
	g.SubDone()
}

// poolHelloConn
var poolHelloConn sync.Pool

func getHelloConn(id int64, stage *Stage, server *helloServer, gate *helloGate, netConn *net.TCPConn) *helloConn {
	var conn *helloConn
	if x := poolHelloConn.Get(); x == nil {
		conn = new(helloConn)
	} else {
		conn = x.(*helloConn)
	}
	conn.onGet(id, stage, server, gate, netConn)
	return conn
}
func putHelloConn(conn *helloConn) {
	conn.onPut()
	poolHelloConn.Put(conn)
}

// helloConn
type helloConn struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	stage   *Stage
	server  *helloServer
	gate    *helloGate
	netConn *net.TCPConn
	// Conn states (zeros)
}

func (c *helloConn) onGet(id int64, stage *Stage, server *helloServer, gate *helloGate, netConn *net.TCPConn) {
	c.id = id
	c.stage = stage
	c.server = server
	c.gate = gate
	c.netConn = netConn
}
func (c *helloConn) onPut() {
	c.stage = nil
	c.server = nil
	c.gate = nil
	c.netConn = nil
}

func (c *helloConn) serve() { // goroutine
	c.netConn.Write([]byte("hello, world!"))
	c.closeConn()
	putHelloConn(c)
}

func (c *helloConn) closeConn() {
	c.netConn.Close()
	c.gate.onConnClosed()
}
