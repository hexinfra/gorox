// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// gRPC framework implementation.

package hemi

import (
	"bytes"
	"errors"
	"sync/atomic"
	"time"
)

func init() {
	RegisterServer("grpcServer", func(name string, stage *Stage) Server {
		s := new(grpcServer)
		s.onCreate(name, stage)
		return s
	})
}

// grpcServer is the gRPC server. A grpcServer has many grpcGates.
type grpcServer struct {
	// Parent
	Server_[*grpcGate]
	// Assocs
	defaultService *Service // default service if not found
	// States
	forServices    []string                // for what services
	exactServices  []*hostnameTo[*Service] // like: ("example.com")
	suffixServices []*hostnameTo[*Service] // like: ("*.example.com")
	prefixServices []*hostnameTo[*Service] // like: ("www.example.*")
	recvTimeout    time.Duration           // timeout to recv the whole message content. zero means no timeout
	sendTimeout    time.Duration           // timeout to send the whole message. zero means no timeout
}

func (s *grpcServer) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)
}

func (s *grpcServer) OnConfigure() {
	s.Server_.OnConfigure()

	// forServices
	s.ConfigureStringList("forServices", &s.forServices, nil, []string{})

	// recvTimeout
	s.ConfigureDuration("recvTimeout", &s.recvTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, 60*time.Second)

	// sendTimeout
	s.ConfigureDuration("sendTimeout", &s.sendTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, 60*time.Second)
}
func (s *grpcServer) OnPrepare() {
	s.Server_.OnPrepare()
}

func (s *grpcServer) BindServices() {
	for _, serviceName := range s.forServices {
		service := s.stage.Service(serviceName)
		if service == nil {
			continue
		}
		service.BindServer(s)
		// TODO: use hash table?
		for _, hostname := range service.exactHostnames {
			s.exactServices = append(s.exactServices, &hostnameTo[*Service]{hostname, service})
		}
		// TODO: use radix trie?
		for _, hostname := range service.suffixHostnames {
			s.suffixServices = append(s.suffixServices, &hostnameTo[*Service]{hostname, service})
		}
		// TODO: use radix trie?
		for _, hostname := range service.prefixHostnames {
			s.prefixServices = append(s.prefixServices, &hostnameTo[*Service]{hostname, service})
		}
	}
}
func (s *grpcServer) findService(hostname []byte) *Service {
	// TODO: use hash table?
	for _, exactMap := range s.exactServices {
		if bytes.Equal(hostname, exactMap.hostname) {
			return exactMap.target
		}
	}
	// TODO: use radix trie?
	for _, suffixMap := range s.suffixServices {
		if bytes.HasSuffix(hostname, suffixMap.hostname) {
			return suffixMap.target
		}
	}
	// TODO: use radix trie?
	for _, prefixMap := range s.prefixServices {
		if bytes.HasPrefix(hostname, prefixMap.hostname) {
			return prefixMap.target
		}
	}
	return nil
}

func (s *grpcServer) Serve() { // runner
	// TODO
}

// grpcGate is a gate of grpcServer.
type grpcGate struct {
	// Parent
	Gate_[*grpcServer]
	// States
}

func (g *grpcGate) onNew(server *grpcServer, id int32) {
	g.Gate_.OnNew(server, id)
}

func (g *grpcGate) Open() error {
	// TODO
	return nil
}
func (g *grpcGate) Shut() error {
	g.MarkShut()
	// TODO // breaks serve()
	return nil
}

func (g *grpcGate) serve() { // runner
	// TODO
}

// grpcConn
type grpcConn struct {
	// Parent
	id      int64
	gate    *grpcGate
	counter atomic.Int64 // can be used to generate a random number
}

func (c *grpcConn) onGet(id int64, gate *grpcGate) {
	c.id = id
	c.gate = gate
}
func (c *grpcConn) onPut() {
	c.gate = nil
	c.counter.Store(0)
}

func (c *grpcConn) IsUDS() bool { return c.gate.IsUDS() }
func (c *grpcConn) IsTLS() bool { return c.gate.IsTLS() }

func (c *grpcConn) MakeTempName(to []byte, unixTime int64) int {
	return makeTempName(to, c.gate.server.Stage().ID(), c.id, unixTime, c.counter.Add(1))
}

//func (c *grpcConn) rpcServer() *grpcServer { return c.server }
