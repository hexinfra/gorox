// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// General RPC Framework implementation.

package hemi

import (
	"errors"
	"time"
)

// RPCServer
type RPCServer interface { // for *hrpcServer and *grpcServer
	// Imports
	Server
	// Methods
	BindServices()
}

// Service is the RPC service.
type Service struct {
	// Parent
	Component_
	// Mixins
	_accessLogger_
	// Assocs
	stage   *Stage      // current stage
	servers []RPCServer // bound rpc servers. may be empty
	// States
	hostnames       [][]byte           // ...
	maxContentSize  int64              // max content size allowed
	exactHostnames  [][]byte           // like: ("example.com")
	suffixHostnames [][]byte           // like: ("*.example.com")
	prefixHostnames [][]byte           // like: ("www.example.*")
	bundlets        map[string]Bundlet // ...
}

func (s *Service) onCreate(compName string, stage *Stage) {
	s.MakeComp(compName)
	s.stage = stage
}
func (s *Service) OnShutdown() { close(s.ShutChan) } // notifies maintain() which shutdown sub components

func (s *Service) OnConfigure() {
	s._accessLogger_.onConfigure(s)

	// .maxContentSize
	s.ConfigureInt64("maxContentSize", &s.maxContentSize, func(value int64) error {
		if value > 0 && value <= _1G {
			return nil
		}
		return errors.New(".maxContentSize has an invalid value")
	}, _16M)
}
func (s *Service) OnPrepare() {
	s._accessLogger_.onPrepare(s)

	initsLock.RLock()
	serviceInit := serviceInits[s.compName]
	initsLock.RUnlock()
	if serviceInit != nil {
		if err := serviceInit(s); err != nil {
			UseExitln(err.Error())
		}
	}
}

func (s *Service) maintain() { // runner
	s.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	s.CloseLog()
	if DebugLevel() >= 2 {
		Printf("service=%s done\n", s.CompName())
	}
	s.stage.DecService()
}

func (s *Service) BindServer(server RPCServer) { s.servers = append(s.servers, server) }
func (s *Service) Servers() []RPCServer        { return s.servers }

/*
func (s *Service) dispatch(exchan) {
	// TODO
}
*/

// Bundlet is a collection of related procedures in a service. A service has many bundlets.
// Bundlets are not components.
type Bundlet interface {
}

// Bundlet_ is a parent.
type Bundlet_ struct { // for all bundlets
}

/*
func (b *Bundlet_) dispatch(exchan) {
}
*/
