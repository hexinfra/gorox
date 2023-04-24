// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC server implementation for gRPC and HRPC.

package internal

// hrpcServer is the HRPC server.
type hrpcServer interface {
	webServer

	linkSvcs()
	findSvc(hostname []byte) *Svc
}

// hrpcRequest
type hrpcRequest interface {
	// TODO
}

// hrpcResponse
type hrpcResponse interface {
	// TODO
}

// GRPCServer is the interface for all gRPC servers.
// Users can implement their own gRPC server in exts, which embeds *grpc.Server and implements the GRPCServer interface.
type GRPCServer interface {
	Server

	LinkSvcs()
	RealServer() any // TODO
}
