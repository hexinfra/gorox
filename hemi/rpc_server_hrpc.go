// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC server implementation.

package hemi

func init() {
	RegisterServer("hrpcServer", func(name string, stage *Stage) Server {
		s := new(hrpcServer)
		s.onCreate(name, stage)
		return s
	})
}

// hrpcServer is the HRPC server.
type hrpcServer struct {
	// Mixins
	rpcServer_[*hrpcGate]
	// States
}

func (s *hrpcServer) onCreate(name string, stage *Stage) {
	s.rpcServer_.onCreate(name, stage)
}
func (s *hrpcServer) OnShutdown() {
	s.rpcServer_.onShutdown()
}

func (s *hrpcServer) OnConfigure() {
	s.rpcServer_.onConfigure(s)
}
func (s *hrpcServer) OnPrepare() {
	s.rpcServer_.onPrepare(s)
}

func (s *hrpcServer) Serve() { // runner
	// TODO
}

// hrpcGate is a gate of hrpcServer.
type hrpcGate struct {
	// Mixins
	Gate_
	// Assocs
	server *hrpcServer
	// States
}

func (g *hrpcGate) init(server *hrpcServer, id int32) {
	g.Gate_.Init(server.stage, id, server.udsMode, server.tlsMode, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *hrpcGate) Open() error {
	// TODO
	return nil
}
func (g *hrpcGate) Shut() error {
	g.MarkShut()
	// TODO
	return nil
}

func (g *hrpcGate) serve() { // runner
	// TODO
}

// hrpcConn
type hrpcConn struct {
	// Mixins
	rpcServerConn_
}

// hrpcExchan is the server-side HRPC exchan.
type hrpcExchan struct {
	// Mixins
	rpcServerExchan_
	// Assocs
	request  hrpcRequest
	response hrpcResponse
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	gate *hrpcGate
	// Exchan states (zeros)
}

// hrpcRequest is the server-side HRPC request.
type hrpcRequest struct {
	// Mixins
	rpcServerRequest_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

// hrpcResponse is the server-side HRPC response.
type hrpcResponse struct {
	// Mixins
	rpcServerResponse_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}
