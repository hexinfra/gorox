// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// SOCKS 5 server.

package socks

import (
	"context"
	"fmt"
	. "github.com/hexinfra/gorox/hemi/internal"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"net"
	"sync"
	"syscall"
)

func init() {
	RegisterServer("socksServer", func(name string, stage *Stage) Server {
		s := new(socksServer)
		s.init(name, stage)
		return s
	})
}

// socksServer
type socksServer struct {
	// Mixins
	Server_
	// Assocs
	// States
	gates []*socksGate
}

func (s *socksServer) init(name string, stage *Stage) {
	s.Server_.Init(name, stage)
}

func (s *socksServer) OnConfigure() {
	s.Server_.OnConfigure()
}
func (s *socksServer) OnPrepare() {
	s.Server_.OnPrepare()
}
func (s *socksServer) OnShutdown() {
	for _, gate := range s.gates {
		gate.shutdown()
	}
	s.Server_.OnShutdown()
}

func (s *socksServer) Serve() { // goroutine
	for id := int32(0); id < s.NumGates(); id++ {
		gate := new(socksGate)
		gate.init(s, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		s.gates = append(s.gates, gate)
		s.IncSub(1)
		go gate.serve()
	}
	s.WaitSubs()
	if Debug(2) {
		fmt.Printf("socksServer=%s done\n", s.Name())
	}
	s.Stage().SubDone()
}

// socksGate
type socksGate struct {
	// Mixins
	Gate_
	// Assocs
	server *socksServer
	// States
	listener *net.TCPListener
}

func (g *socksGate) init(server *socksServer, id int32) {
	g.Gate_.Init(server.Stage(), id, server.Address(), server.MaxConnsPerGate())
	g.server = server
}

func (g *socksGate) open() error {
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
func (g *socksGate) shutdown() error {
	g.Gate_.Shutdown()
	return g.listener.Close()
}

func (g *socksGate) serve() { // goroutine
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
		if err != nil {
			if g.IsShutdown() {
				break
			} else {
				continue
			}
		}
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			socksConn := getSocksConn(connID, g.Stage(), g.server, g, tcpConn)
			go socksConn.serve() // socksConn is put to pool in serve()
			connID++
		}
	}
	// TODO: waiting for all connections end. Use sync.Cond?
	g.server.SubDone()
}

func (g *socksGate) onConnectionClosed() {
	g.DecConns()
}
func (g *socksGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}

// poolSocksConn
var poolSocksConn sync.Pool

func getSocksConn(id int64, stage *Stage, server *socksServer, gate *socksGate, netConn *net.TCPConn) *socksConn {
	var conn *socksConn
	if x := poolSocksConn.Get(); x == nil {
		conn = new(socksConn)
	} else {
		conn = x.(*socksConn)
	}
	conn.onGet(id, stage, server, gate, netConn)
	return conn
}
func putSocksConn(conn *socksConn) {
	conn.onPut()
	poolSocksConn.Put(conn)
}

// socksConn
type socksConn struct {
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	stage   *Stage
	server  *socksServer
	gate    *socksGate
	netConn *net.TCPConn
	// Conn states (zeros)
}

func (c *socksConn) onGet(id int64, stage *Stage, server *socksServer, gate *socksGate, netConn *net.TCPConn) {
	c.id = id
	c.stage = stage
	c.server = server
	c.gate = gate
	c.netConn = netConn
}
func (c *socksConn) onPut() {
	c.stage = nil
	c.server = nil
	c.gate = nil
	c.netConn = nil
}

func (c *socksConn) serve() { // goroutine
	defer putSocksConn(c)

	c.netConn.Write([]byte("not implemented yet"))
	c.closeConn()
}

func (c *socksConn) closeConn() {
	c.netConn.Close()
	c.gate.onConnectionClosed()
}
