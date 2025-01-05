// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP tunnel proxy server.

package tunnel

import (
	"net"

	. "github.com/hexinfra/gorox/hemi"
)

// tunnelServer
type tunnelServer struct {
	// Parent
	Server_[*tunnelGate]
	// Assocs
	// States
}

func (s *tunnelServer) Serve() { // runner
}

// tunnelGate
type tunnelGate struct {
	// Parent
	Gate_[*tunnelServer]
	// States
	listener *net.TCPListener
}

func (g *tunnelGate) Open() error {
	return nil
}
func (g *tunnelGate) Shut() error {
	return nil
}

func (g *tunnelGate) serve() { // runner
}

// tunnelConn
type tunnelConn struct {
}
