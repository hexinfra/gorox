// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General gRPC server implementation.

// Due to gRPC's large dependancies, to keep hemi small, we won't implement our own grpc server.
// Users can implement their own grpc server in exts, which implements GRPCServer interface.

// Maybe we can implements our own gRPC server following its official spec. TBD.

package internal

// GRPCServer is the interface for all grpc servers.
type GRPCServer interface {
	Server
	RealServer() any
	LinkSvc(svc *Svc)
}
