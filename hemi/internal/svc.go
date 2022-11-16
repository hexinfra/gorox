// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Svc and related components. Currently only gRPC and HRPC are planned to support.

package internal

import (
	"fmt"
	"time"
)

// Svc is the microservice.
type Svc struct {
	// Mixins
	Component_
	// Assocs
	stage       *Stage // current stage
	grpcServers []GRPCServer
	hrpcServers []httpServer
	// States
	hostnames       [][]byte // should be used by hrpc only
	exactHostnames  [][]byte // like: ("example.com")
	suffixHostnames [][]byte // like: ("*.example.com")
	prefixHostnames [][]byte // like: ("www.example.*")
}

func (s *Svc) init(name string, stage *Stage) {
	s.CompInit(name)
	s.stage = stage
}

func (s *Svc) OnConfigure() {
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

func (s *Svc) OnShutdown() {
	s.Shutdown()
}

func (s *Svc) LinkGRPC(server GRPCServer) {
	s.grpcServers = append(s.grpcServers, server)
}
func (s *Svc) GRPCServers() []GRPCServer { return s.grpcServers }

func (s *Svc) linkHRPC(server httpServer) {
	s.hrpcServers = append(s.hrpcServers, server)
}

func (s *Svc) maintain() { // goroutine
	Loop(time.Second, s.Shut, func(now time.Time) {
		// TODO
	})
	if Debug(2) {
		fmt.Printf("svc=%s done\n", s.Name())
	}
	s.stage.SubDone()
}

func (s *Svc) dispatchHRPC() {
	// TODO
}
