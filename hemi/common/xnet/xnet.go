// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// X Net.

package xnet

import (
	"net"
)

var (
	DialTimeout = net.DialTimeout
)

type (
	Addr         = net.Addr
	Buffers      = net.Buffers
	Error        = net.Error
	ListenConfig = net.ListenConfig
	Conn         = net.Conn
	TCPConn      = net.TCPConn
	TCPListener  = net.TCPListener
	UDPConn      = net.UDPConn
	UnixConn     = net.UnixConn
)
