// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package rocks

import (
	"fmt"
	. "github.com/hexinfra/gorox/hemi"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func init() {
	RegisterServer("rocksServer", func(name string, stage *Stage) Server {
		s := new(RocksServer)
		s.onCreate(name, stage)
		return s
	})
}

// RocksServer
type RocksServer struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage
	gate  *net.TCPListener
	// States
	address string
	shut    atomic.Bool
	mutex   sync.Mutex
	conns   map[int64]*rockConn
}

func (s *RocksServer) onCreate(name string, stage *Stage) {
	s.CompInit(name)
	s.stage = stage
	s.conns = make(map[int64]*rockConn)
}

func (s *RocksServer) OnConfigure() {
	// address
	if v, ok := s.Find("address"); ok {
		if address, ok := v.String(); ok {
			if p := strings.IndexByte(address, ':'); p == -1 || p == len(address)-1 {
				UseExitln("bad address: " + address)
			} else {
				s.address = address
			}
		} else {
			UseExitln("address should be of string type")
		}
	} else {
		UseExitln("address is required for server")
	}
}
func (s *RocksServer) OnPrepare() {
}

func (s *RocksServer) OnShutdown() {
	s.shut.Store(true)
	s.gate.Close()
}

func (s *RocksServer) Serve() { // goroutine
	addr, err := net.ResolveTCPAddr("tcp", s.address)
	if err != nil {
		EnvExitln(err.Error())
	}
	gate, err := net.ListenTCP("tcp", addr)
	if err != nil {
		EnvExitln(err.Error())
	}
	s.gate = gate
	connID := int64(0)
	for {
		tcpConn, err := s.gate.AcceptTCP()
		if err != nil {
			if s.shut.Load() {
				break
			} else {
				continue
			}
		}
		conn := new(rockConn)
		conn.init(s.stage, s, connID, tcpConn)
		s.addConn(conn)
		go conn.serve()
		connID++
	}
	// TODO: waiting for all connections end. Use sync.Cond?
	if Debug(2) {
		fmt.Printf("rocksServer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *RocksServer) NumConns() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.conns)
}
func (s *RocksServer) addConn(conn *rockConn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.conns[conn.id] = conn
}
func (s *RocksServer) delConn(conn *rockConn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.conns, conn.id)
}

// rockConn
type rockConn struct {
	// Assocs
	stage  *Stage
	server *RocksServer
	// States
	id      int64
	tcpConn *net.TCPConn
}

func (c *rockConn) init(stage *Stage, server *RocksServer, id int64, tcpConn *net.TCPConn) {
	c.stage = stage
	c.server = server
	c.id = id
	c.tcpConn = tcpConn
}

func (c *rockConn) serve() { // goroutine
	defer c.closeConn()
	for i := 0; i < 10; i++ {
		fmt.Fprintf(c.tcpConn, "id=%d\n", c.id)
		time.Sleep(time.Second)
	}
}

func (c *rockConn) closeConn() {
	c.tcpConn.Close()
	c.server.delConn(c)
}
