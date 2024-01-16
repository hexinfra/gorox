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

func (s *hrpcServer) Serve() { // goroutine
}

// hrpcGate is a gate of hrpcServer.
type hrpcGate struct {
	// Mixins
	rpcGate_
}

func (g *hrpcGate) serve() { // goroutine
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
