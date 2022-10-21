// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Net for Linux.

package system

import (
	"syscall"
)

func SetDeferAccept(rawConn syscall.RawConn) (err error) {
	rawConn.Control(func(fd uintptr) {
		err = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_DEFER_ACCEPT, 1)
	})
	return
}

func SetReusePort(rawConn syscall.RawConn) (err error) {
	const SO_REUSEPORT = 0xf // for both amd64 & arm64
	rawConn.Control(func(fd uintptr) {
		err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, SO_REUSEPORT, 1)
	})
	return
}

func SetBuffered(rawConn syscall.RawConn, buffered bool) {
	if buffered {
		rawConn.Control(func(fd uintptr) { syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_CORK, 1) })
	} else {
		rawConn.Control(func(fd uintptr) { syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_CORK, 0) })
	}
}
