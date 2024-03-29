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
	s.rpcServer_.onConfigure()
}
func (s *hrpcServer) OnPrepare() {
	s.rpcServer_.onPrepare()
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
func (g *hrpcGate) _openUnix() error {
	// TODO
	return nil
}
func (g *hrpcGate) _openInet() error {
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
	// Parent
	ServerConn_
}

func (c *hrpcConn) onGet(id int64, gate *hrpcGate) {
	c.ServerConn_.OnGet(id, gate)
}
func (c *hrpcConn) onPut() {
	c.ServerConn_.OnPut()
}

func (c *hrpcConn) rpcServer() rpcServer { return c.Server().(rpcServer) }

// hrpcExchan is the server-side HRPC exchan.
type hrpcExchan struct {
	// Mixins
	_rpcExchan_
	// Assocs
	request  hrpcRequest
	response hrpcResponse
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	gate *hrpcGate
	// Exchan states (zeros)
}

func (x *hrpcExchan) onUse() {
	x._rpcExchan_.onUse()
}
func (x *hrpcExchan) onEnd() {
	x._rpcExchan_.onEnd()
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
