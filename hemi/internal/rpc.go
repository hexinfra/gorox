// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// RPC service and related components. Currently only HRPC and gRPC are planned to support.

package internal

import (
	"time"
)

// GRPCServer is the interface for all grpc servers.
// Due to gRPC's large dependancies, to keep hemi small, we won't implement our own grpc server.
// Users can implement their own grpc server in exts, which embeds *grpc.Server and implements GRPCServer interface.
// Maybe we can implements our own gRPC server following its official spec. TBD.
type GRPCServer interface {
	Server
	RealServer() any
	LinkSvcs()
}

// Svc is the microservice.
type Svc struct {
	// Mixins
	Component_
	// Assocs
	stage       *Stage // current stage
	hrpcServers []httpServer
	grpcServers []GRPCServer
	// States
	hostnames       [][]byte // should be used by hrpc only
	maxContentSize  int64    // max content size allowed
	exactHostnames  [][]byte // like: ("example.com")
	suffixHostnames [][]byte // like: ("*.example.com")
	prefixHostnames [][]byte // like: ("www.example.*")
}

func (s *Svc) onCreate(name string, stage *Stage) {
	s.CompInit(name)
	s.stage = stage
}
func (s *Svc) OnShutdown() {
	close(s.Shut)
}

func (s *Svc) OnConfigure() {
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

func (s *Svc) linkHRPC(server httpServer) { s.hrpcServers = append(s.hrpcServers, server) }
func (s *Svc) LinkGRPC(server GRPCServer) { s.grpcServers = append(s.grpcServers, server) }
func (s *Svc) GRPCServers() []GRPCServer  { return s.grpcServers }

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
