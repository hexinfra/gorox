// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package testee

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterServiceInit("testee", func(service *Service) error {
		/*
			ss := service.Servers()
			for _, s := range ss {
				g := s.GRPCServer().(*grpc.Server)
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
