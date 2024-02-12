// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC incoming message and outgoing message implementation.

// HRPC is a request/response RPC protocol designed for IDC.
// HRPC is under design, its transport protocol is not determined. Maybe we can build it upon QUIC without TLS?

package hemi

// HRPC incoming

func (r *rpcIn_) readContentH() {
	// TODO
}

// HRPC outgoing

func (r *rpcOut_) writeBytesH() {
	// TODO
}
