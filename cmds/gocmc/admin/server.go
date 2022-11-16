// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package admin

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
	RegisterServer("adminServer", func(name string, stage *Stage) Server {
		s := new(AdminServer)
		s.init(name, stage)
		return s
	})
}

// AdminServer
type AdminServer struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage
	gate  *net.TCPListener
	// States
	address  string
	shutdown atomic.Bool
	mutex    sync.Mutex
	conns    map[int64]*adminConn
}

func (s *AdminServer) init(name string, stage *Stage) {
	s.CompInit(name)
	s.stage = stage
	s.conns = make(map[int64]*adminConn)
}

func (s *AdminServer) OnConfigure() {
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
func (s *AdminServer) OnPrepare() {
}
func (s *AdminServer) OnShutdown() {
	s.shutdown.Store(true)
	s.gate.Close()
}

func (s *AdminServer) Serve() { // goroutine
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
			if s.shutdown.Load() {
				break
			} else {
				continue
			}
		}
		conn := new(adminConn)
		conn.init(s.stage, s, connID, tcpConn)
		s.addConn(conn)
		go conn.serve()
		connID++
	}
	// TODO: waiting for all connections end. Use sync.Cond?
	if Debug(2) {
		fmt.Printf("adminServer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *AdminServer) NumConns() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.conns)
}
func (s *AdminServer) addConn(conn *adminConn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.conns[conn.id] = conn
}
func (s *AdminServer) delConn(conn *adminConn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.conns, conn.id)
}

// adminConn
type adminConn struct {
	// Assocs
	stage  *Stage
	server *AdminServer
	// States
	id      int64
	tcpConn *net.TCPConn
}

func (c *adminConn) init(stage *Stage, server *AdminServer, id int64, tcpConn *net.TCPConn) {
	c.stage = stage
	c.server = server
	c.id = id
	c.tcpConn = tcpConn
}

func (c *adminConn) serve() { // goroutine
	defer c.closeConn()
	for i := 0; i < 10; i++ {
		fmt.Fprintf(c.tcpConn, "id=%d\n", c.id)
		time.Sleep(time.Second)
	}
}

func (c *adminConn) closeConn() {
	c.tcpConn.Close()
	c.server.delConn(c)
}
