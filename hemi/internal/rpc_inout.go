// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC incoming and outgoing messages.

package internal

// rpcAgent
type rpcAgent interface {
}

// rpcAgent_
type rpcAgent_ struct {
}

// rpcCall
type rpcCall interface {
}

// rpcCall_
type rpcCall_ struct {
}

// rpcIn
type rpcIn interface {
}

// rpcIn_
type rpcIn_ struct {
	// Assocs
	shell rpcIn
}

// rpcOut
type rpcOut interface {
}

// rpcOut_
type rpcOut_ struct {
	// Assocs
	shell rpcOut
}
