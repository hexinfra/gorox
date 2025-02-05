// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// gRPC types.

// gRPC is a request/response RPC protocol designed by Google.
// gRPC is based on HTTP/2: https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md

package hemi

// _grpcHolder_
type _grpcHolder_ struct { // for grpcClient, grpcServer, and grpcGate
}

func (h *_grpcHolder_) onConfigure(comp Component) {
}
func (h *_grpcHolder_) onPrepare(comp Component) {
}

// grpcConn_ is a parent.
type grpcConn_ struct { // for grpcConn and gConn
}

// grpcCall_ is a parent.
type grpcCall_ struct { // for grpcCall and gCall
}
