// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC incoming and outgoing messages implementation.

package hemi

import (
	"sync/atomic"
	"time"
)

// rpcBroker
type rpcBroker interface {
	// TODO
}

// rpcBroker_
type rpcBroker_ struct {
	// TODO
}

func (b *rpcBroker_) onConfigure(shell Component, sendTimeout time.Duration, recvTimeout time.Duration) {
}
func (b *rpcBroker_) onPrepare(shell Component) {
}

// rpcConn
type rpcConn interface {
	// TODO
}

// rpcConn_
type rpcConn_ struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	usedCalls atomic.Int32 // num of streams served or used
	broken    atomic.Bool  // is conn broken?
}

func (c *rpcConn_) onGet() {
}
func (c *rpcConn_) onPut() {
}

// rpcCall is the interface for *hrpcCall and *HRCall.
type rpcCall interface {
	// TODO
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

// rpcIn is the interface for *hrpcReq and *HResp. Used as shell by rpcIn_.
type rpcIn interface {
	// TODO
}

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

// rpcOut is the interface for *hrpcResp and *HReq. Used as shell by rpcOut_.
type rpcOut interface {
	// TODO
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
