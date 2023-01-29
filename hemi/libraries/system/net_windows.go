// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Net for Windows.

package system

import (
	"syscall"
)

func SetDeferAccept(rawConn syscall.RawConn) (err error) {
	return
}

func SetReusePort(rawConn syscall.RawConn) (err error) {
	rawConn.Control(func(fd uintptr) {
		err = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})
	return
}

func SetBuffered(rawConn syscall.RawConn, buffered bool) {
	// Not supported? Do nothing.
}
