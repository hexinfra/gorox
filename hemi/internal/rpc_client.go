// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC client implementation.

package internal

// rpcClient
type rpcClient interface {
	// Imports
	client
	// Methods
}

// rpcOutgate_
type rpcOutgate_ struct {
	// Mixins
	outgate_
	// States
}

func (f *rpcOutgate_) onCreate(name string, stage *Stage) {
	f.outgate_.onCreate(name, stage)
}

func (f *rpcOutgate_) onConfigure(shell Component) {
	f.outgate_.onConfigure()
	if f.tlsConfig != nil {
		f.tlsConfig.InsecureSkipVerify = true
	}
}
func (f *rpcOutgate_) onPrepare(shell Component) {
	f.outgate_.onPrepare()
}

// rpcBackend_
type rpcBackend_ struct {
}

// rpcNode_
type rpcNode_ struct {
}

// clientCall_
type clientCall_ struct {
	// Mixins
	rpcCall_
}

// clientReq is the client-side RPC request.
type clientReq interface {
}

// clientReq_
type clientReq_ struct {
	// Mixins
	rpcOut_
}

// clientResp is the client-side RPC response.
type clientResp interface {
}

// clientResp_
type clientResp_ struct {
	// Mixins
	rpcIn_
}
