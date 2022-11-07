// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General server implementation.

package internal

import (
	"strings"
)

// Server component.
type Server interface {
	Component
	Serve() // goroutine
}

// Server_ is the mixin for all servers.
type Server_ struct {
	// Mixins
	office_
	// States
	colonPort      string // like: ":9876"
	colonPortBytes []byte // like: []byte(":9876")
}

func (s *Server_) Init(name string, stage *Stage) {
	s.office_.init(name, stage)
}

func (s *Server_) OnConfigure() {
	s.office_.onConfigure()
	p := strings.IndexByte(s.address, ':')
	s.colonPort = s.address[p:]
	s.colonPortBytes = []byte(s.colonPort)
}
func (s *Server_) OnPrepare() {
	s.office_.onPrepare()
}
func (s *Server_) OnShutdown() {
	s.office_.onShutdown()
}

func (s *Server_) ColonPort() string      { return s.colonPort }
func (s *Server_) ColonPortBytes() []byte { return s.colonPortBytes }
