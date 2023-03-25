// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Net for FreeBSD.

package system

import (
	"syscall"
)

func SetDeferAccept(rawConn syscall.RawConn) (err error) {
	return
}

func SetReusePort(rawConn syscall.RawConn) (err error) {
	// A maximum of	256 processes can share	one socket.
	const SO_REUSEPORT_LB = 0x10000 // for 386, amd64, arm, arm64, riscv64
	rawConn.Control(func(fd uintptr) {
		err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, SO_REUSEPORT_LB, 1)
	})
	return
}

func SetBuffered(rawConn syscall.RawConn, buffered bool) {
	// TODO: tcp nopush
}
