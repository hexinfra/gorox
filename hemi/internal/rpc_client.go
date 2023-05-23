// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC client implementation.

package internal

// rpcClient
type rpcClient interface {
}

// rpcOutgate_
type rpcOutgate_ struct {
}

// rpcBackend_
type rpcBackend_ struct {
}

// rpcNode_
type rpcNode_ struct {
}

// clientExchan_
type clientExchan_ struct {
	// Mixins
	rpcExchan_
}

// clientReq is the client-side RPC request.
type clientReq interface {
}

// clientReq_
type clientReq_ struct {
	// Mixins
	rpcOut_
}

// clientResp is the client-side RPC response.
type clientResp interface {
}

// clientResp_
type clientResp_ struct {
	// Mixins
	rpcIn_
}
