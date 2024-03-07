// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC client implementation.

package hemi

// rpcClientConn
type rpcClientConn interface {
	Close() error
}

// rpcClientExchan
type rpcClientExchan interface {
}

// rpcClientRequest_
type rpcClientRequest_ struct {
	// Parent
	rpcOut_
	// TODO
}

// rpcClientResponse_
type rpcClientResponse_ struct {
	// Parent
	rpcIn_
	// TODO
}
