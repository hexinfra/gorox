// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is a hello svc showing how to use Gorox RPC server to host a svc.

package hello

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	// Register svc initializer.
	RegisterSvcInit("hello", func(svc *Svc) error {
		/*
			servers := svc.GRPCServers()
			for _, server := range servers {
				g := server.RealServer().(*grpc.Server)
				pb.RegisterGreeterServer(g, &greetServer{})
			}
		*/
		return nil
	})
}

/*
type greetServer struct {
	pb.UnimplementedGreeterServer
}

func (s *greetServer) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	return &pb.HelloReply{Message: "Hello " + in.GetName()}, nil
}
*/
