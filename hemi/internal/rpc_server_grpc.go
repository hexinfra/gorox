// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// gRPC server bridge.

package internal

// GRPCBridge is the interface for all gRPC server bridges.
// Users can implement their own gRPC server in exts, which may embeds *grpc.Server and must implements the GRPCBridge interface.
type GRPCBridge interface {
	// Imports
	rpcServer
	// Methods
	GRPCServer() any // may be a *grpc.Server
}
