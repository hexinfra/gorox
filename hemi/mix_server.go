// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// General server for net, rpc, and web.

package hemi

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Server component. A Server has a group of Gates.
type Server interface {
	// Imports
	Component
	// Methods
	Serve()           // runner
	holder() _holder_ // used by gates to copy the configs
}

// Server_ is a parent.
type Server_[G Gate] struct { // for all servers
	// Parent
	Component_
	// Mixins
	_holder_ // to carry configs used by gates
	// Assocs
	gates []G // a server has many gates
	// States
	colonport         string // like: ":9876"
	colonportBytes    []byte // []byte(colonport)
	udsColonport      string // uds doesn't have a port. we can use this as its colonport if server is listening at uds
	udsColonportBytes []byte // []byte(udsColonport)
	numGates          int32  // number of gates
}

func (s *Server_[G]) OnCreate(compName string, stage *Stage) {
	s.MakeComp(compName)
	s.stage = stage
}
func (s *Server_[G]) OnShutdown() {
	// We don't use close(s.ShutChan) to notify gates as gates are blocking on accept().
	for _, gate := range s.gates {
		gate.Shut() // this causes gate to close and return immediately
	}
}

func (s *Server_[G]) OnConfigure() {
	s._holder_.onConfigure(s, 60*time.Second, 60*time.Second)

	// .address
	if v, ok := s.Find("address"); ok {
		if address, ok := v.String(); ok && address != "" {
			if p := strings.IndexByte(address, ':'); p == -1 {
				s.udsMode = true
			} else {
				s.colonport = address[p:]
			}
			s.address = address
		} else {
			UseExitln(".address should be of string type")
		}
	} else {
		UseExitln(".address is required for servers")
	}

	if s.udsMode {
		// .udsColonport
		s.ConfigureString("udsColonport", &s.udsColonport, nil, ":80")
	}

	// .numGates
	s.ConfigureInt32("numGates", &s.numGates, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".numGates has an invalid value")
	}, s.stage.NumCPU())
}
func (s *Server_[G]) OnPrepare() {
	s._holder_.onPrepare(s)

	if s.udsMode {
		s.udsColonportBytes = []byte(s.udsColonport)
		s.numGates = 1 // unix domain socket does not support reuseaddr/reuseport.
	} else {
		s.colonportBytes = []byte(s.colonport)
	}
}

func (s *Server_[G]) NumGates() int32 { return s.numGates }
func (s *Server_[G]) AddGate(gate G) { // Serve() calls this to append gates
	s.gates = append(s.gates, gate)
	s.subs.Add(1)
}

func (s *Server_[G]) Colonport() string {
	if s.udsMode {
		return s.udsColonport
	} else {
		return s.colonport
	}
}
func (s *Server_[G]) ColonportBytes() []byte {
	if s.udsMode {
		return s.udsColonportBytes
	} else {
		return s.colonportBytes
	}
}

func (s *Server_[G]) DecGate()   { s.subs.Done() }
func (s *Server_[G]) WaitGates() { s.subs.Wait() }

func (s *Server_[G]) holder() _holder_ { return s._holder_ } // for copying configs

// Gate is the interface for all gates. Gates are not components.
type Gate interface {
	// Imports
	holder
	// Methods
	Shut() error
	IsShut() bool
}

// Gate_ is a parent.
type Gate_[S Server] struct { // for all gates
	// Mixins
	_holder_
	// Assocs
	server S
	// States
	id   int32          // gate id
	shut atomic.Bool    // is gate shut?
	subs sync.WaitGroup // sub conns to wait for
}

func (g *Gate_[S]) OnNew(server S, id int32) {
	g._holder_ = server.holder()
	g.server = server
	g.id = id
	g.shut.Store(false)
}

func (g *Gate_[S]) Server() S { return g.server }

func (g *Gate_[S]) ID() int32    { return g.id }
func (g *Gate_[S]) MarkShut()    { g.shut.Store(true) }
func (g *Gate_[S]) IsShut() bool { return g.shut.Load() }

func (g *Gate_[S]) IncConn()   { g.subs.Add(1) }
func (g *Gate_[S]) DecConn()   { g.subs.Done() }
func (g *Gate_[S]) WaitConns() { g.subs.Wait() }
