// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package test

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterSvcInit("test", func(svc *Svc) error {
		/*
			ss := svc.GRPCServers()
			for _, s := range ss {
				g := s.RealServer().(*grpc.Server)
				pb.RegisterGreeterServer(g, &greetService{})
			}
		*/
		return nil
	})
}

/*
type greetService struct {
	pb.UnimplementedGreeterServer
}

func (s *greetService) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	return &pb.HelloReply{Message: "Hello " + in.GetName()}, nil
}
*/
