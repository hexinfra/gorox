// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// gRPC server implementation.

package internal

// GRPCServer is the interface for all gRPC servers.
// Due to gRPC's large dependancies, to keep Hemi small, we won't implement our own gRPC server.
// Users can implement their own gRPC server in exts, which embeds *grpc.Server and implements the GRPCServer interface.
// Maybe we can implement our own gRPC server following its official spec. TBD.
type GRPCServer interface {
	Server

	LinkSvcs()
	RealServer() any
}
