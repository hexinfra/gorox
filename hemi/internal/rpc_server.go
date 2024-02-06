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
type rpcServer_ struct {
	// Mixins
	Server_
	rpcBroker_
	// Assocs
	gates          []Gate
	defaultService *Service // default service if not found
	// States
	forServices    []string                // for what services
	exactServices  []*hostnameTo[*Service] // like: ("example.com")
	suffixServices []*hostnameTo[*Service] // like: ("*.example.com")
	prefixServices []*hostnameTo[*Service] // like: ("www.example.*")
}

func (s *rpcServer_) onConfigure(shell Component) {
	s.Server_.OnConfigure()

	// forServices
	s.ConfigureStringList("forServices", &s.forServices, nil, []string{})
}
func (s *rpcServer_) onPrepare(shell Component) {
	s.Server_.OnPrepare()
}

func (s *rpcServer_) BindServices() {
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
func (s *rpcServer_) findService(hostname []byte) *Service {
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

// serverWire_
type serverWire_ struct {
	// Mixins
	rpcWire_
}

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
