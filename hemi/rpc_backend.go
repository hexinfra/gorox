// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC backend implementation.

package hemi

import (
	"time"
)

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
	b.rpcBroker_.onConfigure(shell, 60*time.Second, 60*time.Second)
}
func (b *rpcBackend_[N]) onPrepare(shell Component, numNodes int) {
	b.Backend_.onPrepare()
	b.rpcBroker_.onPrepare(shell)
}

// backendRPCConn_
type backendRPCConn_ struct {
	Conn_
	rpcConn_
	backend rpcBackend
}

func (c *backendRPCConn_) onGet(id int64, udsMode bool, tlsMode bool, backend rpcBackend) {
	c.Conn_.onGet(id, udsMode, tlsMode, time.Now().Add(backend.AliveTimeout()))
	c.rpcConn_.onGet()
	c.backend = backend
}
func (c *backendRPCConn_) onPut() {
	c.backend = nil
	c.Conn_.onPut()
	c.rpcConn_.onPut()
}

func (c *backendRPCConn_) rpcBackend() rpcBackend { return c.backend }

// backendCall_
type backendCall_ struct {
	// Mixins
	rpcCall_
	// TODO
}

// backendReq_
type backendReq_ struct {
	// Mixins
	rpcOut_
	// TODO
}

// backendResp_
type backendResp_ struct {
	// Mixins
	rpcIn_
	// TODO
}
