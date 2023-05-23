// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC incoming and outgoing messages.

package internal

// rpcKeeper
type rpcKeeper interface {
}

// rpcKeeper_
type rpcKeeper_ struct {
}

// rpcCall is the interface for *hrpcCall and *HCall.
type rpcCall interface {
}

// rpcCall_ is the mixin for hrpcCall and HCall.
type rpcCall_ struct {
}

// rpcIn is the interface for *hrpcReq and *HResp. Used as shell by rpcIn_.
type rpcIn interface {
}

// rpcIn_ is a mixin for serverReq_ and clientResp_.
type rpcIn_ struct {
	// Assocs
	shell rpcIn
}

// rpcOut is the interface for *hrpcResp and *HReq. Used as shell by rpcOut_.
type rpcOut interface {
}

// rpcOut_ is a mixin for serverResp_ and clientReq_.
type rpcOut_ struct {
	// Assocs
	shell rpcOut
}
