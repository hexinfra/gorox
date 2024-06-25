// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HRPC server implementation.

package hemi

import (
	"bytes"
	"errors"
	"sync/atomic"
	"time"
)

func init() {
	RegisterServer("hrpcServer", func(name string, stage *Stage) Server {
		s := new(hrpcServer)
		s.onCreate(name, stage)
		return s
	})
}

// hrpcServer is the HRPC server.
type hrpcServer struct {
	// Parent
	Server_[*hrpcGate]
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

func (s *hrpcServer) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)
}

func (s *hrpcServer) OnConfigure() {
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
func (s *hrpcServer) OnPrepare() {
	s.Server_.OnPrepare()
}

func (s *hrpcServer) BindServices() {
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
func (s *hrpcServer) findService(hostname []byte) *Service {
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

func (s *hrpcServer) Serve() { // runner
	// TODO
}

// hrpcGate is a gate of hrpcServer.
type hrpcGate struct {
	// Parent
	Gate_
	// Assocs
	server *hrpcServer
	// States
}

func (g *hrpcGate) init(id int32, server *hrpcServer) {
	g.Gate_.Init(id, server.MaxConnsPerGate())
	g.server = server
}

func (g *hrpcGate) Server() Server  { return g.server }
func (g *hrpcGate) Address() string { return g.server.Address() }
func (g *hrpcGate) IsTLS() bool     { return g.server.IsTLS() }
func (g *hrpcGate) IsUDS() bool     { return g.server.IsUDS() }

func (g *hrpcGate) Open() error {
	// TODO
	return nil
}
func (g *hrpcGate) Shut() error {
	g.MarkShut()
	// TODO // breaks serve()
	return nil
}

func (g *hrpcGate) serve() { // runner
	// TODO
}

// hrpcConn
type hrpcConn struct {
	// Parent
	id      int64
	gate    *hrpcGate
	counter atomic.Int64 // can be used to generate a random number
}

func (c *hrpcConn) onGet(id int64, gate *hrpcGate) {
	c.id = id
	c.gate = gate
}
func (c *hrpcConn) onPut() {
	c.gate = nil
	c.counter.Store(0)
}

func (c *hrpcConn) IsTLS() bool { return c.gate.IsTLS() }
func (c *hrpcConn) IsUDS() bool { return c.gate.IsUDS() }

func (c *hrpcConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.gate.server.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

//func (c *hrpcConn) rpcServer() *hrpcServer { return c.server }
