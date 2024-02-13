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
	// Mixins
	rpcBackend_[*hrpcNode]
	// States
}

func (b *HRPCBackend) onCreate(name string, stage *Stage) {
	b.rpcBackend_.onCreate(name, stage, b.NewNode)
}

func (b *HRPCBackend) OnConfigure() {
	b.rpcBackend_.onConfigure(b)
}
func (b *HRPCBackend) OnPrepare() {
	b.rpcBackend_.onPrepare(b, len(b.nodes))
}

func (b *HRPCBackend) NewNode(id int32) *hrpcNode {
	node := new(hrpcNode)
	node.init(id, b)
	return node
}

// hrpcNode
type hrpcNode struct {
	// Mixins
	Node_
	// Assocs
	// States
}

func (n *hrpcNode) init(id int32, backend *HRPCBackend) {
	n.Node_.Init(id, backend)
}

func (n *hrpcNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("hrpcNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

// HConn
type HConn struct {
	rpcBackendConn_
}

func (c *HConn) Close() error { // only used by clients of dial
	// TODO
	return nil
}

// HExchan is the backend-side HRPC exchan.
type HExchan struct {
	// Mixins
	rpcBackendExchan_
	// Assocs
	request  HRequest
	response HResponse
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	node *hrpcNode
	id   int32
	// Exchan states (zeros)
}

// HRequest is the backend-side HRPC request.
type HRequest struct {
	// Mixins
	rpcBackendRequest_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

// HResponse is the backend-side HRPC response.
type HResponse struct {
	// Mixins
	rpcBackendResponse_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}
