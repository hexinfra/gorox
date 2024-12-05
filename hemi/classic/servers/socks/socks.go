// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// SOCKS 5 proxy server.

// Don't confuse SOCKS with sock. We use "sock" as an abbreviation of "webSocket".

package socks

import (
	"context"
	"net"
	"sync"
	"syscall"

	"github.com/hexinfra/gorox/hemi/library/system"

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
		s.IncSub() // gate
		go gate.serve()
	}
	s.WaitSubs() // gates
	if DebugLevel() >= 2 {
		Printf("socksServer=%s done\n", s.Name())
	}
	s.Stage().DecSub() // server
}

func (s *socksServer) serveConn(conn *socksConn) { // runner
	defer putSocksConn(conn)

	// -> ver(1) nmethods(1) methods(1-255)
	// <- ver(1) method(1) : 00=noAuth 01=gssapi 02=username/password ff=noAcceptableMethods
	// -> ver(1) cmd(1) rsv(1) atyp(1) dstAddr(v) dstPort(2)
	// <- ver(1) res(1) rsv(1) atyp(1) bndAddr(v) bndPort(2)
	conn.tcpConn.Write([]byte("not implemented yet"))
	conn.Close()
}

// socksGate
type socksGate struct {
	// Parent
	Gate_
	// Assocs
	server *socksServer
	// States
	listener *net.TCPListener
}

func (g *socksGate) init(id int32, server *socksServer) {
	g.Gate_.Init(id, server.MaxConnsPerGate())
	g.server = server
}

func (g *socksGate) Server() Server  { return g.server }
func (g *socksGate) Address() string { return g.server.Address() }
func (g *socksGate) IsTLS() bool     { return g.server.IsTLS() }
func (g *socksGate) IsUDS() bool     { return g.server.IsUDS() }

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
		g.IncConn()
		if actives := g.IncActives(); g.ReachLimit(actives) {
			g.justClose(tcpConn)
			continue
		}
		socksConn := getSocksConn(connID, g, tcpConn)
		go g.server.serveConn(socksConn) // socksConn is put to pool in serve()
		connID++
	}
	g.WaitConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("socksGate=%d done\n", g.ID())
	}
	g.server.DecSub() // gate
}

func (g *socksGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.DecActives()
	g.DecConn()
}

// socksConn
type socksConn struct {
	// Conn states (stocks)
	stockInput [8192]byte
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	gate    *socksGate
	tcpConn *net.TCPConn
	input   []byte
	// Conn states (zeros)
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

func (c *socksConn) onGet(id int64, gate *socksGate, tcpConn *net.TCPConn) {
	c.id = id
	c.gate = gate
	c.tcpConn = tcpConn
	c.input = c.stockInput[:]
}
func (c *socksConn) onPut() {
	c.input = nil
	c.tcpConn = nil
	c.gate = nil
}

func (c *socksConn) Close() {
	c.tcpConn.Close()
	c.gate.DecActives()
	c.gate.DecConn()
}
