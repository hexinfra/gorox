// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC server implementation.

package internal

// hrpcServer
type hrpcServer struct {
	// Mixins
	rpcServer_
	// States
}

func (s *hrpcServer) Serve() { // goroutine
}

// hrpcGate
type hrpcGate struct {
	// Mixins
	rpcGate_
}

func (g *hrpcGate) serve() { // goroutine
}

// hrpcCall
type hrpcCall struct {
	// Mixins
	serverCall_
	// Assocs
	req  hrpcReq
	resp hrpcResp
}

// hrpcReq
type hrpcReq struct {
	// TODO
}

// hrpcResp
type hrpcResp struct {
	// TODO
}
