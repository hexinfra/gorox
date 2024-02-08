// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC incoming and outgoing messages implementation.

package hemi

import (
	"time"
)

// rpcBroker_
type rpcBroker_ struct {
	// TODO
}

func (b *rpcBroker_) onConfigure(shell Component, sendTimeout time.Duration, recvTimeout time.Duration) {
}
func (b *rpcBroker_) onPrepare(shell Component) {
}

// rpcConn_
type rpcConn_ struct {
}

func (c *rpcConn_) onGet() {
}
func (c *rpcConn_) onPut() {
}

// rpcCall_ is the mixin for hrpcCall and HCall.
type rpcCall_ struct {
	// TODO
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	region      Region    // a region-based memory pool
}

func (c *rpcCall_) onUse() {
	c.region.Init()
}
func (c *rpcCall_) onEnd() {
	c.region.Free()
}

func (c *rpcCall_) buffer256() []byte          { return c.stockBuffer[:] }
func (c *rpcCall_) unsafeMake(size int) []byte { return c.region.Make(size) }

// rpcIn_ is the mixin for serverReq_ and backendResp_.
type rpcIn_ struct {
	// Assocs
	shell rpcIn
	// TODO
	rpcIn0
}
type rpcIn0 struct {
	arrayKind int8 // kind of current r.array. see arrayKindXXX
}

// rpcOut_ is the mixin for serverResp_ and backendReq_.
type rpcOut_ struct {
	// Assocs
	shell rpcOut
	// TODO
	rpcOut0
}
type rpcOut0 struct {
}
