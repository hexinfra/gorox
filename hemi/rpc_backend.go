// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC backend implementation.

package hemi

import (
	"time"
)

// rpcBackend
type rpcBackend interface {
	// Imports
	Backend
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
	b.rpcBroker_.onConfigure(shell, 60*time.Second, 60*time.Second)
}
func (b *rpcBackend_[N]) onPrepare(shell Component, numNodes int) {
	b.Backend_.onPrepare()
	b.rpcBroker_.onPrepare(shell)
}

// rpcBackendConn_
type rpcBackendConn_ struct {
	// Mixins
	BackendConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
}

func (c *rpcBackendConn_) onGet(id int64, backend rpcBackend, node Node) {
	c.BackendConn_.onGet(id, backend, node)
}
func (c *rpcBackendConn_) onPut() {
	c.BackendConn_.onPut()
}

func (c *rpcBackendConn_) Backend() rpcBackend { return c.backend.(rpcBackend) }

// rpcBackendExchan_
type rpcBackendExchan_ struct {
	// Mixins
	Stream_
	// TODO
}

func (x *rpcBackendExchan_) onUse() {
	x.Stream_.onUse()
}
func (x *rpcBackendExchan_) onEnd() {
	x.Stream_.onEnd()
}

// rpcBackendRequest_
type rpcBackendRequest_ struct {
	// Mixins
	rpcOut_
	// TODO
}

// rpcBackendResponse_
type rpcBackendResponse_ struct {
	// Mixins
	rpcIn_
	// TODO
}
