// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC incoming and outgoing messages.

package internal

// rpcBroker
type rpcBroker interface {
	// TODO
}

// rpcBroker_
type rpcBroker_ struct {
	// TODO
}

// rpcCall is the interface for *hrpcCall and *HCall.
type rpcCall interface {
	// TODO
}

// rpcCall_ is the mixin for hrpcCall and HCall.
type rpcCall_ struct {
	// TODO
}

// rpcIn is the interface for *hrpcReq and *HResp. Used as shell by rpcIn_.
type rpcIn interface {
	// TODO
}

// rpcIn_ is the mixin for serverReq_ and clientResp_.
type rpcIn_ struct {
	// Assocs
	shell rpcIn
	// TODO
}

// rpcOut is the interface for *hrpcResp and *HReq. Used as shell by rpcOut_.
type rpcOut interface {
	// TODO
}

// rpcOut_ is the mixin for serverResp_ and clientReq_.
type rpcOut_ struct {
	// Assocs
	shell rpcOut
	// TODO
}
