// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// RPC service and related components.

package hemi

import (
	"errors"
	"time"
)

// Service is the RPC service.
type Service struct {
	// Mixins
	Component_
	// Assocs
	stage   *Stage      // current stage
	stater  Stater      // the stater which is used by this service
	servers []rpcServer // bound rpc servers. may be empty
	// States
	hostnames       [][]byte           // ...
	accessLog       *logcfg            // ...
	logger          *logger            // service access logger
	maxContentSize  int64              // max content size allowed
	exactHostnames  [][]byte           // like: ("example.com")
	suffixHostnames [][]byte           // like: ("*.example.com")
	prefixHostnames [][]byte           // like: ("www.example.*")
	bundlets        map[string]Bundlet // ...
}

func (s *Service) onCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
}
func (s *Service) OnShutdown() {
	close(s.ShutChan) // notifies maintain()
}

func (s *Service) OnConfigure() {
	// withStater
	if v, ok := s.Find("withStater"); ok {
		if name, ok := v.String(); ok && name != "" {
			if stater := s.stage.Stater(name); stater == nil {
				UseExitf("unknown stater: '%s'\n", name)
			} else {
				s.stater = stater
			}
		} else {
			UseExitln("invalid withStater")
		}
	}

	// maxContentSize
	s.ConfigureInt64("maxContentSize", &s.maxContentSize, func(value int64) error {
		if value > 0 && value <= _1G {
			return nil
		}
		return errors.New(".maxContentSize has an invalid value")
	}, _16M)
}
func (s *Service) OnPrepare() {
	if s.accessLog != nil {
		//s.logger = newLogger(s.accessLog.logFile, s.accessLog.rotate)
	}

	initsLock.RLock()
	serviceInit := serviceInits[s.name]
	initsLock.RUnlock()
	if serviceInit != nil {
		if err := serviceInit(s); err != nil {
			UseExitln(err.Error())
		}
	}
}

func (s *Service) Log(str string) {
	if s.logger != nil {
		s.logger.Log(str)
	}
}
func (s *Service) Logln(str string) {
	if s.logger != nil {
		s.logger.Logln(str)
	}
}
func (s *Service) Logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Logf(format, args...)
	}
}

func (s *Service) BindServer(server rpcServer) { s.servers = append(s.servers, server) }
func (s *Service) Servers() []rpcServer        { return s.servers }

func (s *Service) maintain() { // runner
	s.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Printf("service=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *Service) dispatch(req rpcServerRequest, resp rpcServerResponse) {
	// TODO
}

// Bundlet is a bundle of procedures in a Service. A Service may has many Bundlets.
// Bundlets are not components.
type Bundlet interface {
}

// Bundlet_ is the mixin for all bundlets.
type Bundlet_ struct {
}

func (b *Bundlet_) dispatch(req rpcServerRequest, resp rpcServerResponse) {
}

// rpcServerRequest is the server-side RPC request.
type rpcServerRequest interface {
	Service() *Service
	// TODO
}

// rpcServerResponse is the server-side RPC response.
type rpcServerResponse interface {
	Req() rpcServerRequest
	// TODO
}

// rpcBackendRequest is the backend-side RPC request.
type rpcBackendRequest interface {
}

// rpcBackendResponse is the backend-side RPC response.
type rpcBackendResponse interface {
}
