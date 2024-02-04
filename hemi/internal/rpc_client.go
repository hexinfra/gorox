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

func (n *rpcNode_) setTLSMode() {
	n.Node_.setTLSMode()
	n.tlsConfig.InsecureSkipVerify = true
}

// clientCall_
type clientCall_ struct {
	// Mixins
	Conn_
	rpcCall_
	// Call states (stocks)
	// Call states (controlled)
	// Call states (non-zeros)
	client rpcClient
	// Call states (zeros)
}

func (c *clientCall_) onGet(id int64, udsMode bool, tlsMode bool, client rpcClient) {
	c.Conn_.onGet(id, udsMode, tlsMode, time.Now().Add(client.AliveTimeout()))
	// c.rpcCall_.onGet()?
	c.client = client
}
func (c *clientCall_) onPut() {
	c.client = nil
	c.Conn_.onPut()
	// c.rpcCall_.onPut()?
}

func (c *clientCall_) getClient() rpcClient { return c.client }

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
