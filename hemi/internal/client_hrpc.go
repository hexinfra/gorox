// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC client implementation.

package internal

// hrpcOutgate
type hrpcOutgate struct {
}

// hrpcBackend
type hrpcBackend struct {
}

// hrpcNode
type hrpcNode struct {
}

// HCall is the client-side HRPC call.
type HCall struct {
	// Mixins
	clientCall_
	// Assocs
	req  HReq
	resp HResp
	// Call states (stocks)
	// Call states (controlled)
	// Call states (non-zeros)
	node *hrpcNode
	id   int32
	// Call states (zeros)
}

// HReq is the client-side HRPC request.
type HReq struct {
	// Mixins
	clientReq_
	// Call states (stocks)
	// Call states (controlled)
	// Call states (non-zeros)
	// Call states (zeros)
}

// HResp is the client-side HRPC response.
type HResp struct {
	// Mixins
	clientResp_
	// Call states (stocks)
	// Call states (controlled)
	// Call states (non-zeros)
	// Call states (zeros)
}
