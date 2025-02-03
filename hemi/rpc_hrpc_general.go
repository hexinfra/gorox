// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HRPC types.

// HRPC is a request/response RPC protocol designed for IDC.
// HRPC is under design, its transport protocol is not determined. Maybe we can build it upon HTTP/3 without TLS?

package hemi

// _hrpcHolder_
type _hrpcHolder_ struct { // for hrpcClient, hrpcServer, and hrpcGate
}

func (h *_hrpcHolder_) onConfigure(comp Component) {
}
func (h *_hrpcHolder_) onPrepare(comp Component) {
}

// hrpcConn_ is a parent.
type hrpcConn_ struct { // for hrpcConn and hConn
}

// hrpcCall_ is a parent.
type hrpcCall_ struct { // for hrpcCall and hCall
}
