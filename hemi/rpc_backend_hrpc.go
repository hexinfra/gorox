// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC backend implementation.

package hemi

import (
	"time"
)

func init() {
	RegisterBackend("hrpcBackend", func(name string, stage *Stage) Backend {
		b := new(HRPCBackend)
		b.onCreate(name, stage)
		return b
	})
}

// HRPCBackend
type HRPCBackend struct {
	// Parent
	rpcBackend_[*hrpcNode]
	// States
}

func (b *HRPCBackend) onCreate(name string, stage *Stage) {
	b.rpcBackend_.onCreate(name, stage)
}

func (b *HRPCBackend) OnConfigure() {
	b.rpcBackend_.onConfigure(b)
}
func (b *HRPCBackend) OnPrepare() {
	b.rpcBackend_.onPrepare(b)
}

func (b *HRPCBackend) CreateNode(name string) Node {
	node := new(hrpcNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

// hrpcNode
type hrpcNode struct {
	// Parent
	Node_
	// Assocs
	// States
}

func (n *hrpcNode) onCreate(name string, backend *HRPCBackend) {
	n.Node_.OnCreate(name, backend)
}

func (n *hrpcNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *hrpcNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *hrpcNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("hrpcNode=%s done\n", n.name)
	}
	n.backend.DecSub()
}

// HConn
type HConn struct {
	// Parent
	BackendConn_
}

func (c *HConn) rpcBackend() rpcBackend { return c.Backend().(rpcBackend) }

func (c *HConn) Close() error {
	return nil
}

// HExchan is the backend-side HRPC exchan.
type HExchan struct {
	// Mixins
	_rpcExchan_
	// Assocs
	request  HRequest
	response HResponse
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	id int32
	// Exchan states (zeros)
}

func (x *HExchan) onUse() {
	x._rpcExchan_.onUse()
}
func (x *HExchan) onEnd() {
	x._rpcExchan_.onEnd()
}

// HRequest is the backend-side HRPC request.
type HRequest struct {
	// Parent
	rpcBackendRequest_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

// HResponse is the backend-side HRPC response.
type HResponse struct {
	// Parent
	rpcBackendResponse_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}
