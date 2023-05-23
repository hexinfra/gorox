// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC server implementation.

package internal

import (
	"bytes"
)

// GRPCBridge is the interface for all gRPC server bridges.
// Users can implement their own gRPC server in exts, which may embeds *grpc.Server and must implements the GRPCBridge interface.
type GRPCBridge interface {
	rpcServer

	GRPCServer() any // may be a *grpc.Server
}

// ThriftBridge is the interface for all Thrift server bridges.
// Users can implement their own Thrift server in exts, which may embeds thrift.TServer and must implements the ThriftBridge interface.
type ThriftBridge interface {
	rpcServer

	ThriftServer() any // may be a thrift.TServer?
}

// rpcServer
type rpcServer interface {
	Server

	LinkSvcs()
}

// rpcServer_
type rpcServer_ struct {
	// Mixins
	Server_
	// Assocs
	gates      []rpcGate
	defaultSvc *Svc // default svc if not found
	// States
	forSvcs    []string            // for what svcs
	exactSvcs  []*hostnameTo[*Svc] // like: ("example.com")
	suffixSvcs []*hostnameTo[*Svc] // like: ("*.example.com")
	prefixSvcs []*hostnameTo[*Svc] // like: ("www.example.*")
}

func (s *rpcServer_) onConfigure(shell Component) {
	s.Server_.OnConfigure()

	// forSvcs
	s.ConfigureStringList("forSvcs", &s.forSvcs, nil, []string{})
}
func (s *rpcServer_) onPrepare(shell Component) {
	s.Server_.OnPrepare()
}

func (s *rpcServer_) LinkSvcs() {
	for _, svcName := range s.forSvcs {
		svc := s.stage.Svc(svcName)
		if svc == nil {
			continue
		}
		svc.LinkServer(s.shell.(rpcServer))
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

// serverExchan_
type serverExchan_ struct {
	// Mixins
	rpcExchan_
}

// Req is the server-side RPC request.
type Req interface {
	Svc() *Svc
}

// serverReq_
type serverReq_ struct {
	// Mixins
	rpcIn_
}

// Resp is the server-side RPC response.
type Resp interface {
	Req() Req
}

// serverResp_
type serverResp_ struct {
	// Mixins
	rpcOut_
}
