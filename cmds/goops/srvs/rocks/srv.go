// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
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
type RocksServer struct { // implements hemi.Server
	// Mixins
	Component_
	// Assocs
	stage *Stage
	gate  *net.TCPListener
	// States
	address        string
	colonPort      string
	colonPortBytes []byte
	readTimeout    time.Duration
	writeTimeout   time.Duration
	shut           atomic.Bool
	mutex          sync.Mutex
	conns          map[int64]*goroxConn
}

func (s *RocksServer) onCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
	s.conns = make(map[int64]*goroxConn)
}
func (s *RocksServer) OnShutdown() {
	s.shut.Store(true)
	s.gate.Close()
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
	p := strings.IndexByte(s.address, ':')
	s.colonPort = s.address[:p]
	s.colonPortBytes = []byte(s.colonPort)
	// readTimeout
	s.ConfigureDuration("readTimeout", &s.readTimeout, func(value time.Duration) bool { return value > 0 }, 60*time.Second)
	// writeTimeout
	s.ConfigureDuration("writeTimeout", &s.writeTimeout, func(value time.Duration) bool { return value > 0 }, 60*time.Second)
}
func (s *RocksServer) OnPrepare() {
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
		s.IncSub(1)
		conn := new(goroxConn)
		conn.init(s.stage, s, connID, tcpConn)
		s.addConn(conn)
		go conn.serve()
		connID++
	}
	s.WaitSubs() // conns. TODO: max timeout?
	if IsDebug(2) {
		Debugf("rocksServer=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *RocksServer) Stage() *Stage               { return s.stage }
func (s *RocksServer) ColonPort() string           { return s.colonPort }
func (s *RocksServer) ColonPortBytes() []byte      { return s.colonPortBytes }
func (s *RocksServer) TLSMode() bool               { return false }
func (s *RocksServer) ReadTimeout() time.Duration  { return s.readTimeout }
func (s *RocksServer) WriteTimeout() time.Duration { return s.writeTimeout }

func (s *RocksServer) NumConns() int { // used by apps
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return len(s.conns)
}
func (s *RocksServer) addConn(conn *goroxConn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.conns[conn.id] = conn
}
func (s *RocksServer) delConn(conn *goroxConn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.conns, conn.id)
}

// A goroxConn is a connection with a gorox leader instance.
type goroxConn struct {
	// Assocs
	stage  *Stage
	server *RocksServer
	// States
	id      int64
	tcpConn *net.TCPConn
}

func (c *goroxConn) init(stage *Stage, server *RocksServer, id int64, tcpConn *net.TCPConn) {
	c.stage = stage
	c.server = server
	c.id = id
	c.tcpConn = tcpConn
}

func (c *goroxConn) serve() { // goroutine
	defer c.closeConn()
	for i := 0; i < 10; i++ {
		fmt.Fprintf(c.tcpConn, "id=%d\n", c.id)
	}
}

func (c *goroxConn) closeConn() {
	c.tcpConn.Close()
	c.server.delConn(c)
	c.server.SubDone()
}
