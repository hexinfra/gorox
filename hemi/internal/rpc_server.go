// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC server implementation.

package internal

import (
	"bytes"
)

// rpcServer_
type rpcServer_[G Gate] struct {
	// Mixins
	Server_[G]
	rpcBroker_
	// Assocs
	defaultService *Service // default service if not found
	// States
	forServices    []string                // for what services
	exactServices  []*hostnameTo[*Service] // like: ("example.com")
	suffixServices []*hostnameTo[*Service] // like: ("*.example.com")
	prefixServices []*hostnameTo[*Service] // like: ("www.example.*")
}

func (s *rpcServer_[G]) onConfigure(shell Component) {
	s.Server_.OnConfigure()

	// forServices
	s.ConfigureStringList("forServices", &s.forServices, nil, []string{})
}
func (s *rpcServer_[G]) onPrepare(shell Component) {
	s.Server_.OnPrepare()
}

func (s *rpcServer_[G]) BindServices() {
	for _, serviceName := range s.forServices {
		service := s.stage.Service(serviceName)
		if service == nil {
			continue
		}
		service.BindServer(s.shell.(rpcServer))
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
func (s *rpcServer_[G]) findService(hostname []byte) *Service {
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

// serverRPCConn_
type serverRPCConn_ struct {
	// Mixins
	rpcConn_
	id     int64
	server rpcServer
	gate   Gate
}

func (c *serverRPCConn_) onGet(id int64, server rpcServer, gate Gate) {
	c.rpcConn_.onGet()
	c.id = id
	c.server = server
	c.gate = gate
}
func (c *serverRPCConn_) onPut() {
	c.server = nil
	c.gate = nil
	c.rpcConn_.onPut()
}

func (c *serverRPCConn_) rpcServer() rpcServer { return c.server }

// serverCall_
type serverCall_ struct {
	// Mixins
	rpcCall_
	// TODO
}

// serverReq_
type serverReq_ struct {
	// Mixins
	rpcIn_
	// TODO
}

// serverResp_
type serverResp_ struct {
	// Mixins
	rpcOut_
	// TODO
}
