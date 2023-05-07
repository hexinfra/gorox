// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC server implementation.

package internal

import (
	"bytes"
)

// RPCServer
type RPCServer interface {
	Server

	LinkSvcs()
}

// rpcServer_
type rpcServer_ struct {
	// Mixins
	Server_
	// Assocs
	gates      []rpcGate
	defaultSvc *Svc
	// States
	forSvcs    []string            // for what svcs
	exactSvcs  []*hostnameTo[*Svc] // like: ("example.com")
	suffixSvcs []*hostnameTo[*Svc] // like: ("*.example.com")
	prefixSvcs []*hostnameTo[*Svc] // like: ("www.example.*")
}

func (s *rpcServer_) onConfigure() {
	// forSvcs
	s.ConfigureStringList("forSvcs", &s.forSvcs, nil, []string{})
}

func (s *rpcServer_) LinkSvcs() {
	for _, svcName := range s.forSvcs {
		svc := s.stage.Svc(svcName)
		if svc == nil {
			continue
		}
		svc.LinkServer(s.shell.(RPCServer))
		// TODO: use hash table?
		for _, hostname := range svc.exactHostnames {
			s.exactSvcs = append(s.exactSvcs, &hostnameTo[*Svc]{hostname, svc})
		}
		// TODO: use radix trie?
		for _, hostname := range svc.suffixHostnames {
			s.suffixSvcs = append(s.suffixSvcs, &hostnameTo[*Svc]{hostname, svc})
		}
		// TODO: use radix trie?
		for _, hostname := range svc.prefixHostnames {
			s.prefixSvcs = append(s.prefixSvcs, &hostnameTo[*Svc]{hostname, svc})
		}
	}
}
func (s *rpcServer_) findSvc(hostname []byte) *Svc {
	// TODO: use hash table?
	for _, exactMap := range s.exactSvcs {
		if bytes.Equal(hostname, exactMap.hostname) {
			return exactMap.target
		}
	}
	// TODO: use radix trie?
	for _, suffixMap := range s.suffixSvcs {
		if bytes.HasSuffix(hostname, suffixMap.hostname) {
			return suffixMap.target
		}
	}
	// TODO: use radix trie?
	for _, prefixMap := range s.prefixSvcs {
		if bytes.HasPrefix(hostname, prefixMap.hostname) {
			return prefixMap.target
		}
	}
	return nil
}

// rpcGate
type rpcGate interface {
	Gate
}

// rpcGate_
type rpcGate_ struct {
	// Mixins
	Gate_
}

// Ping
type Ping interface {
	Svc() *Svc
}

// Pong
type Pong interface {
	Ping() Ping
}
