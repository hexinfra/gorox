// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HRPC framework (server and client) implementation.

// HRPC is a request/response RPC protocol designed for IDC.
// HRPC is under design, its transport protocol is not determined. Maybe we can build it upon HTTP/3 without TLS?

package hemi

import (
	"bytes"
	"errors"
	"sync/atomic"
	"time"
)

//////////////////////////////////////// HRPC holder implementation ////////////////////////////////////////

// _hrpcHolder_
type _hrpcHolder_ struct {
}

// hrpcConn_
type hrpcConn_ struct {
}

// hrpcCall_
type hrpcCall_ struct {
}

//////////////////////////////////////// HRPC server implementation ////////////////////////////////////////

func init() {
	RegisterServer("hrpcServer", func(name string, stage *Stage) Server {
		s := new(hrpcServer)
		s.onCreate(name, stage)
		return s
	})
}

// hrpcServer is the HRPC server. An hrpcServer has many hrpcGates.
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
	Gate_[*hrpcServer]
	// States
}

func (g *hrpcGate) onNew(server *hrpcServer, id int32) {
	g.Gate_.OnNew(server, id)
}

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
	// States
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

func (c *hrpcConn) IsUDS() bool { return c.gate.IsUDS() }
func (c *hrpcConn) IsTLS() bool { return c.gate.IsTLS() }

func (c *hrpcConn) MakeTempName(to []byte, unixTime int64) int {
	return makeTempName(to, c.gate.server.Stage().ID(), c.id, unixTime, c.counter.Add(1))
}

//func (c *hrpcConn) rpcServer() *hrpcServer { return c.server }

// hrpcCall
type hrpcCall struct {
	// request
	// response
}

//////////////////////////////////////// HRPC client implementation ////////////////////////////////////////

// hrpcClient
type hrpcClient struct {
}

// hrpcNode
type hrpcNode struct {
}

// hConn
type hConn struct {
}

// hCall
type hCall struct {
}
