// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC client implementation.

package internal

import (
	"time"
)

// rpcClient
type rpcClient interface {
	// Imports
	_client
	streamHolder
	contentSaver
	// Methods
}

// rpcOutgate_
type rpcOutgate_ struct {
	// Mixins
	outgate_
	rpcBroker_
	streamHolder_
	contentSaver_
	// States
}

func (f *rpcOutgate_) onCreate(name string, stage *Stage) {
	f.outgate_.onCreate(name, stage)
}

func (f *rpcOutgate_) onConfigure(shell Component) {
	f.outgate_.onConfigure()
	f.rpcBroker_.onConfigure(shell, 60*time.Second, 60*time.Second)
	f.streamHolder_.onConfigure(shell, 1000)
	f.contentSaver_.onConfigure(shell, TmpsDir()+"/rpc/outgates/"+shell.Name())
}
func (f *rpcOutgate_) onPrepare(shell Component) {
	f.outgate_.onPrepare()
	f.rpcBroker_.onPrepare(shell)
	f.streamHolder_.onPrepare(shell)
	f.contentSaver_.onPrepare(shell, 0755)
}

// rpcBackend_
type rpcBackend_[N Node] struct {
	// Mixins
	Backend_[N]
	rpcBroker_
	// States
	health any // TODO
}

func (b *rpcBackend_[N]) onCreate(name string, stage *Stage, creator interface{ createNode(id int32) N }) {
	b.Backend_.onCreate(name, stage, creator)
}

func (b *rpcBackend_[N]) onConfigure(shell Component) {
	b.Backend_.onConfigure()
	if b.tlsConfig != nil {
		b.tlsConfig.InsecureSkipVerify = true
	}
}
func (b *rpcBackend_[N]) onPrepare(shell Component, numNodes int) {
	b.Backend_.onPrepare()
}

// rpcNode_
type rpcNode_ struct {
	// Mixins
	Node_
	// States
}

func (n *rpcNode_) init(id int32) {
	n.Node_.init(id)
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
