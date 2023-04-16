// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// RPC service and related components. Currently only HRPC and gRPC are planned to support.

package internal

import (
	"time"
)

// rpcServer
type rpcServer interface {
	LinkSvcs()
}

// Svc is the RPC service.
type Svc struct {
	// Mixins
	Component_
	// Assocs
	stage       *Stage        // current stage
	stater      Stater        // the stater which is used by this svc
	hrpcServers []*hrpcServer // linked hrpc servers. may be empty
	grpcServers []GRPCServer  // linked grpc servers. may be empty
	// States
	hostnames       [][]byte // should be used by HRPC only
	maxContentSize  int64    // max content size allowed
	exactHostnames  [][]byte // like: ("example.com")
	suffixHostnames [][]byte // like: ("*.example.com")
	prefixHostnames [][]byte // like: ("www.example.*")
}

func (s *Svc) onCreate(name string, stage *Stage) {
	s.MakeComp(name)
	s.stage = stage
}
func (s *Svc) OnShutdown() {
	close(s.Shut)
}

func (s *Svc) OnConfigure() {
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
	s.ConfigureInt64("maxContentSize", &s.maxContentSize, func(value int64) bool { return value > 0 && value <= _1G }, _16M)
}
func (s *Svc) OnPrepare() {
	initsLock.RLock()
	svcInit := svcInits[s.name]
	initsLock.RUnlock()
	if svcInit != nil {
		if err := svcInit(s); err != nil {
			UseExitln(err.Error())
		}
	}
}

func (s *Svc) linkHRPC(server *hrpcServer) { s.hrpcServers = append(s.hrpcServers, server) }
func (s *Svc) LinkGRPC(server GRPCServer)  { s.grpcServers = append(s.grpcServers, server) }

func (s *Svc) GRPCServers() []GRPCServer { return s.grpcServers }

func (s *Svc) maintain() { // goroutine
	Loop(time.Second, s.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugf("svc=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *Svc) dispatchHRPC() {
	// TODO
}
