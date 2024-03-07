// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC client implementation.

package hemi

// HConn is the client-side HRPC connection.
type HConn struct {
}

func (c *HConn) Close() error {
	return nil
}

// HExchan is the client-side HRPC exchan.
type HExchan struct {
	// Mixins
	_rpcExchan_
	// Assocs
	request  HRequest
	response HResponse
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	id int32
	// Exchan states (zeros)
}

func (x *HExchan) onUse() {
	x._rpcExchan_.onUse()
}
func (x *HExchan) onEnd() {
	x._rpcExchan_.onEnd()
}

// HRequest is the client-side HRPC request.
type HRequest struct {
	// Parent
	rpcClientRequest_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}

// HResponse is the client-side HRPC response.
type HResponse struct {
	// Parent
	rpcClientResponse_
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	// Exchan states (zeros)
}
