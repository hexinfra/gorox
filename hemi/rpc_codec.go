// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC incoming and outgoing messages implementation.

package hemi

// rpcIn is the interface for *hrpcRequest and *HResponse. Used as shell by rpcIn_.
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

// rpcOut is the interface for *hrpcResponse and *HRequest. Used as shell by rpcOut_.
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
