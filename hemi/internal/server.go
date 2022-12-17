// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General server implementation.

package internal

import (
	"crypto/tls"
	"strings"
	"sync/atomic"
	"time"
)

// Server component.
type Server interface {
	Component
	Serve() // goroutine

	Stage() *Stage
	ColonPortBytes() []byte
	TLSMode() bool
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
}

// Server_ is the mixin for all servers.
type Server_ struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	// States
	address         string        // hostname:port
	colonPort       string        // like: ":9876"
	colonPortBytes  []byte        // like: []byte(":9876")
	tlsMode         bool          // tls mode?
	tlsConfig       *tls.Config   // set if is tls mode
	readTimeout     time.Duration // read() timeout
	writeTimeout    time.Duration // write() timeout
	numGates        int32         // number of gates
	maxConnsPerGate int32         // max concurrent connections allowed per gate
}

func (s *Server_) OnCreate(name string, stage *Stage) {
	s.CompInit(name)
	s.stage = stage
}

func (s *Server_) OnConfigure() {
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
		UseExitln("address is required for servers")
	}
	p := strings.IndexByte(s.address, ':')
	s.colonPort = s.address[p:]
	s.colonPortBytes = []byte(s.colonPort)
	// tlsMode
	s.ConfigureBool("tlsMode", &s.tlsMode, false)
	if s.tlsMode {
		s.tlsConfig = new(tls.Config)
	}
	// readTimeout
	s.ConfigureDuration("readTimeout", &s.readTimeout, func(value time.Duration) bool { return value > 0 }, 60*time.Second)
	// writeTimeout
	s.ConfigureDuration("writeTimeout", &s.writeTimeout, func(value time.Duration) bool { return value > 0 }, 60*time.Second)
	// numGates
	s.ConfigureInt32("numGates", &s.numGates, func(value int32) bool { return value > 0 }, s.stage.NumCPU())
	// maxConnsPerGate
	s.ConfigureInt32("maxConnsPerGate", &s.maxConnsPerGate, func(value int32) bool { return value > 0 }, 100000)
}
func (s *Server_) OnPrepare() {
}

func (s *Server_) Stage() *Stage               { return s.stage }
func (s *Server_) Address() string             { return s.address }
func (s *Server_) ColonPort() string           { return s.colonPort }
func (s *Server_) ColonPortBytes() []byte      { return s.colonPortBytes }
func (s *Server_) TLSMode() bool               { return s.tlsMode }
func (s *Server_) ReadTimeout() time.Duration  { return s.readTimeout }
func (s *Server_) WriteTimeout() time.Duration { return s.writeTimeout }
func (s *Server_) NumGates() int32             { return s.numGates }
func (s *Server_) MaxConnsPerGate() int32      { return s.maxConnsPerGate }

// Gate_ is a mixin for mesher gates and server gates.
type Gate_ struct {
	// Mixins
	waiter_
	// Assocs
	stage *Stage // current stage
	// States
	id       int32
	address  string
	shut     atomic.Bool
	maxConns int32
	numConns atomic.Int32 // TODO: false sharing
}

func (g *Gate_) Init(stage *Stage, id int32, address string, maxConns int32) {
	g.stage = stage
	g.id = id
	g.address = address
	g.maxConns = maxConns
	g.numConns.Store(0)
}

func (g *Gate_) Stage() *Stage   { return g.stage }
func (g *Gate_) Address() string { return g.address }

func (g *Gate_) MarkShut()    { g.shut.Store(true) }
func (g *Gate_) IsShut() bool { return g.shut.Load() }

func (g *Gate_) DecConns() int32  { return g.numConns.Add(-1) }
func (g *Gate_) ReachLimit() bool { return g.numConns.Add(1) > g.maxConns }
