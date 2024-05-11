// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC incoming and outgoing messages implementation.

package hemi

// rpcConn
type rpcConn interface {
	// TODO
}

// _rpcConn_
type _rpcConn_ struct {
}

// rpcExchan is the interface for *hrpcExchan and *HExchan.
type rpcExchan interface {
	buffer256() []byte
	unsafeMake(size int) []byte
}

// _rpcExchan_
type _rpcExchan_ struct {
	// Exchan states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	region Region // a region-based memory pool
	// Exchan states (zeros)
}

func (x *_rpcExchan_) onUse() {
	x.region.Init()
}
func (x *_rpcExchan_) onEnd() {
	x.region.Free()
}

func (x *_rpcExchan_) buffer256() []byte          { return x.stockBuffer[:] }
func (x *_rpcExchan_) unsafeMake(size int) []byte { return x.region.Make(size) }

// rpcIn_ is the parent for rpcServerRequest_ and rpcClientResponse_.
type rpcIn_ struct {
	// Assocs
	shell interface { // *hrpcRequest, *HResponse
		// TODO
	}
	// TODO
	rpcIn0
}
type rpcIn0 struct {
	arrayKind int8 // kind of current r.array. see arrayKindXXX
}

// rpcOut_ is the parent for rpcServerResponse_ and rpcClientRequest_.
type rpcOut_ struct {
	// Assocs
	shell interface { // *hrpcResponse, *HRequest
		// TODO
	}
	// TODO
	rpcOut0
}
type rpcOut0 struct {
}
