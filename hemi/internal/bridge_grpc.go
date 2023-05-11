// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// gRPC server bridge.

package internal

// GRPCBridge is the interface for all gRPC server bridges.
// Users can implement their own gRPC server bridge in exts, which embeds *grpc.Server and implements the GRPCBridge interface.
type GRPCBridge interface {
	RPCServer

	GRPCServer() any // must be a *grpc.Server
}
