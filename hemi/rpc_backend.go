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

func (b *rpcBackend_[N]) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
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

// rpcNode_
type rpcNode_ struct {
	// Parent
	Node_
	// Assocs
	// States
}

func (n *rpcNode_) onCreate(name string, backend Backend) {
	n.Node_.OnCreate(name, backend)
}

func (n *rpcNode_) onConfigure() {
	n.Node_.OnConfigure()
}
func (n *rpcNode_) onPrepare() {
	n.Node_.OnPrepare()
}

// rpcBackendConn
type rpcBackendConn interface {
	Close() error
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
	// Stream states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Stream states (controlled)
	// Stream states (non-zeros)
	region Region // a region-based memory pool
	// Stream states (zeros)
}

func (x *rpcBackendExchan_) onUse() {
	x.region.Init()
}
func (x *rpcBackendExchan_) onEnd() {
	x.region.Free()
}

func (x *rpcBackendExchan_) buffer256() []byte          { return x.stockBuffer[:] }
func (x *rpcBackendExchan_) unsafeMake(size int) []byte { return x.region.Make(size) }

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
