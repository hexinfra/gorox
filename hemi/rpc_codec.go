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
	counter     atomic.Int64 // can be used to generate a random number
	usedExchans atomic.Int32 // num of exchans served or used
	broken      atomic.Bool  // is conn broken?
}

func (c *rpcConn_) onGet() {
}
func (c *rpcConn_) onPut() {
}

// rpcExchan is the interface for *hrpcExchan and *HExchan.
type rpcExchan interface {
	// TODO
}

// rpcExchan_ is the mixin for hrpcExchan and HExchan.
type rpcExchan_ struct {
	// TODO
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	region      Region    // a region-based memory pool
}

func (x *rpcExchan_) onUse() {
	x.region.Init()
}
func (x *rpcExchan_) onEnd() {
	x.region.Free()
}

func (x *rpcExchan_) buffer256() []byte          { return x.stockBuffer[:] }
func (x *rpcExchan_) unsafeMake(size int) []byte { return x.region.Make(size) }

// rpcIn is the interface for *hrpcReq and *HResp. Used as shell by rpcIn_.
type rpcIn interface {
	// TODO
}

// rpcIn_ is the mixin for rpcServerRequest_ and rpcBackendResponse_.
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

// rpcOut_ is the mixin for rpcServerResponse_ and rpcBackendRequest_.
type rpcOut_ struct {
	// Assocs
	shell rpcOut
	// TODO
	rpcOut0
}
type rpcOut0 struct {
}
