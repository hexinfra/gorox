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
type rpcBackend_[N rpcNode] struct {
	// Parent
	Backend_[N]
	// Mixins
	_rpcAgent_
	_loadBalancer_
	// States
	health any // TODO
}

func (b *rpcBackend_[N]) onCreate(name string, stage *Stage, newNode func(id int32) N) {
	b.Backend_.OnCreate(name, stage, newNode)
	b._loadBalancer_.init()
}

func (b *rpcBackend_[N]) onConfigure(shell Component) {
	b.Backend_.OnConfigure()
	b._rpcAgent_.onConfigure(shell, 60*time.Second, 60*time.Second)
	b._loadBalancer_.onConfigure(shell)
}
func (b *rpcBackend_[N]) onPrepare(shell Component) {
	b.Backend_.OnPrepare()
	b._rpcAgent_.onPrepare(shell)
	b._loadBalancer_.onPrepare(len(b.nodes))
}

// rpcNode
type rpcNode interface {
	Node
}

// rpcBackendConn_
type rpcBackendConn_ struct {
	// Parent
	BackendConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
}

func (c *rpcBackendConn_) onGet(id int64, node Node) {
	c.BackendConn_.OnGet(id, node)
}
func (c *rpcBackendConn_) onPut() {
	c.BackendConn_.OnPut()
}

func (c *rpcBackendConn_) rpcBackend() rpcBackend { return c.Backend().(rpcBackend) }

// rpcBackendExchan_
type rpcBackendExchan_ struct {
	// Parent
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
	// Parent
	rpcOut_
	// TODO
}

// rpcBackendResponse_
type rpcBackendResponse_ struct {
	// Parent
	rpcIn_
	// TODO
}
