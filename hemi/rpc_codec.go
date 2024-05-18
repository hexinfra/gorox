// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC incoming message and outgoing message implementation.

// HRPC is a request/response RPC protocol designed for IDC.
// HRPC is under design, its transport protocol is not determined. Maybe we can build it upon QUIC without TLS?

package hemi

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
