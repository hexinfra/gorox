// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC server implementation.

package internal

// hrpcServer is the HRPC server.
type hrpcServer struct {
	// Mixins
	rpcServer_
	// States
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
	g.Gate_.Init(server.stage, id, server.address, server.maxConnsPerGate)
	g.server = server
}

func (g *hrpcGate) open() error {
	// TODO
	return nil
}
func (g *hrpcGate) shut() error {
	g.MarkShut()
	// TODO
	return nil
}

func (g *hrpcGate) serve() { // runner
	// TODO
}

// hrpcCall is the server-side HRPC call.
type hrpcCall struct {
	// Mixins
	serverCall_
	// Assocs
	req  hrpcReq
	resp hrpcResp
	// Call states (stocks)
	// Call states (controlled)
	// Call states (non-zeros)
	gate *hrpcGate
	// Call states (zeros)
}

// hrpcReq is the server-side HRPC request.
type hrpcReq struct {
	// Mixins
	serverReq_
	// Call states (stocks)
	// Call states (controlled)
	// Call states (non-zeros)
	// Call states (zeros)
}

// hrpcResp is the server-side HRPC response.
type hrpcResp struct {
	// Mixins
	serverResp_
	// Call states (stocks)
	// Call states (controlled)
	// Call states (non-zeros)
	// Call states (zeros)
}
