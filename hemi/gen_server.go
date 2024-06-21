// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// General server and gate for net, rpc, and web.

package hemi

import (
	"crypto/tls"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Server component. A Server is a group of gates.
type Server interface {
	// Imports
	Component
	// Methods
	Serve() // runner
}

// Server_ is the parent for all servers.
type Server_[G Gate] struct {
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
	gates []G    // a server has many gates
	// States
	readTimeout       time.Duration // read() timeout
	writeTimeout      time.Duration // write() timeout
	address           string        // hostname:port, /path/to/unix.sock
	colonPort         string        // like: ":9876"
	colonPortBytes    []byte        // []byte(colonPort)
	tlsMode           bool          // use tls to secure the transport?
	tlsConfig         *tls.Config   // set if tls mode is true
	udsMode           bool          // is address a unix domain socket?
	udsColonPort      string        // uds doesn't have a port. use this as port if server is listening at uds
	udsColonPortBytes []byte        // []byte(udsColonPort)
	maxConnsPerGate   int32         // max concurrent connections allowed per gate
	numGates          int32         // number of gates
}

func (s *Server_[G]) OnCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
}
func (s *Server_[G]) OnShutdown() {
	// We don't use close(s.ShutChan) to notify gates.
	for _, gate := range s.gates {
		gate.Shut() // this causes gate to close and return immediately
	}
}

func (s *Server_[G]) OnConfigure() {
	// readTimeout
	s.ConfigureDuration("readTimeout", &s.readTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".readTimeout has an invalid value")
	}, 60*time.Second)

	// writeTimeout
	s.ConfigureDuration("writeTimeout", &s.writeTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".writeTimeout has an invalid value")
	}, 60*time.Second)

	// address
	if v, ok := s.Find("address"); ok {
		if address, ok := v.String(); ok && address != "" {
			if p := strings.IndexByte(address, ':'); p == -1 {
				s.udsMode = true
			} else {
				s.colonPort = address[p:]
				s.colonPortBytes = []byte(s.colonPort)
			}
			s.address = address
		} else {
			UseExitln("address should be of string type")
		}
	} else {
		UseExitln(".address is required for servers")
	}

	// udsColonPort
	s.ConfigureString("udsColonPort", &s.udsColonPort, nil, ":80")
	s.udsColonPortBytes = []byte(s.udsColonPort)

	// tlsMode
	s.ConfigureBool("tlsMode", &s.tlsMode, false)
	if s.tlsMode {
		s.tlsConfig = new(tls.Config)
	}

	// maxConnsPerGate
	s.ConfigureInt32("maxConnsPerGate", &s.maxConnsPerGate, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxConnsPerGate has an invalid value")
	}, 10000)

	// numGates
	s.ConfigureInt32("numGates", &s.numGates, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".numGates has an invalid value")
	}, s.stage.NumCPU())
}
func (s *Server_[G]) OnPrepare() {
	if s.udsMode { // unix domain socket does not support reuseaddr/reuseport.
		s.numGates = 1
	}
}

func (s *Server_[G]) AddGate(gate G)  { s.gates = append(s.gates, gate) }
func (s *Server_[G]) NumGates() int32 { return s.numGates }

func (s *Server_[G]) Stage() *Stage               { return s.stage }
func (s *Server_[G]) ReadTimeout() time.Duration  { return s.readTimeout }
func (s *Server_[G]) WriteTimeout() time.Duration { return s.writeTimeout }
func (s *Server_[G]) Address() string             { return s.address }
func (s *Server_[G]) ColonPort() string {
	if s.udsMode {
		return s.udsColonPort
	} else {
		return s.colonPort
	}
}
func (s *Server_[G]) ColonPortBytes() []byte {
	if s.udsMode {
		return s.udsColonPortBytes
	} else {
		return s.colonPortBytes
	}
}
func (s *Server_[G]) IsTLS() bool            { return s.tlsMode }
func (s *Server_[G]) TLSConfig() *tls.Config { return s.tlsConfig }
func (s *Server_[G]) IsUDS() bool            { return s.udsMode }
func (s *Server_[G]) MaxConnsPerGate() int32 { return s.maxConnsPerGate }

// Gate is the interface for all gates. Gates are not components.
type Gate interface {
	// Methods
	Server() Server
	Address() string
	ID() int32
	IsTLS() bool
	IsUDS() bool
	Open() error
	Shut() error
	IsShut() bool
	OnConnClosed()
}

// Gate_ is the parent for all gates.
type Gate_ struct {
	// States
	id       int32          // gate id
	maxConns int32          // max concurrent conns allowed
	shut     atomic.Bool    // is gate shut?
	numConns atomic.Int32   // TODO: false sharing
	subs     sync.WaitGroup // sub conns to wait for
}

func (g *Gate_) Init(id int32, maxConns int32) {
	g.id = id
	g.maxConns = maxConns
	g.shut.Store(false)
	g.numConns.Store(0)
}

func (g *Gate_) ID() int32        { return g.id }
func (g *Gate_) IsShut() bool     { return g.shut.Load() }
func (g *Gate_) MarkShut()        { g.shut.Store(true) }
func (g *Gate_) DecConns() int32  { return g.numConns.Add(-1) }
func (g *Gate_) ReachLimit() bool { return g.numConns.Add(1) > g.maxConns }

func (g *Gate_) IncSub()   { g.subs.Add(1) }
func (g *Gate_) WaitSubs() { g.subs.Wait() }
func (g *Gate_) DecSub()   { g.subs.Done() }

func (g *Gate_) OnConnClosed() {
	g.DecConns()
	g.DecSub()
}
