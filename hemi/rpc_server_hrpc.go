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
	// Parent
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
	// Parent
	Gate_
	// Assocs
	// States
}

func (g *hrpcGate) init(id int32, server *hrpcServer) {
	g.Gate_.Init(id, server)
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

// hrpcStream is the server-side HRPC stream.
type hrpcStream struct {
	// Mixins
	rpcServerStream_
	// Assocs
	request  hrpcRequest
	response hrpcResponse
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	gate *hrpcGate
	// Stream states (zeros)
}

// hrpcRequest is the server-side HRPC request.
type hrpcRequest struct {
	// Mixins
	rpcServerRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

// hrpcResponse is the server-side HRPC response.
type hrpcResponse struct {
	// Mixins
	rpcServerResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}
