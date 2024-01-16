// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Net for Linux.

package system

import (
	"syscall"
	"unsafe"
)

func SetDeferAccept(rawConn syscall.RawConn) (err error) {
	rawConn.Control(func(fd uintptr) {
		err = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_DEFER_ACCEPT, 1)
	})
	return
}

func SetReusePort(rawConn syscall.RawConn) (err error) {
	const SO_REUSEPORT = 0xf // for amd64, arm64, riscv64, loong64
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

var (
	tcpInfoSize = unsafe.Sizeof(TCPInfo{})
)

// TCPState TCP FSM state
// link: /usr/include/netinet/tcp.h
type TCPState uint8

const (
	_ TCPState = iota
	TCPStateEstablished
	TCPStateSynSent
	TCPStateSynRcvd
	TCPStateFinWait1
	TCPStateFinWait2
	TCPStateTimeWait
	TCPStateClosed
	TCPStateCloseWait
	TCPStateLastAck
	TCPStateListen
	TCPStateClosing
)

// TCPInfo TCP statistics for a specified socket.
// link: /usr/include/netinet/tcp.h
type TCPInfo struct {
	State TCPState
}

func (t *TCPInfo) IsEstablished() bool {
	return t.State == TCPStateEstablished
}

func (t *TCPInfo) CanWrite() bool {
	return t.IsEstablished() || t.State == TCPStateCloseWait
}

func GetTCPInfo(sockfd uintptr) (*TCPInfo, error) {
	ti := &TCPInfo{}
	retCode, _, err := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		uintptr(sockfd),
		uintptr(syscall.SOL_TCP),
		uintptr(syscall.TCP_INFO),
		uintptr(unsafe.Pointer(ti)),
		uintptr(unsafe.Pointer(&tcpInfoSize)),
		0,
	)

	if retCode != 0 {
		return nil, err
	}

	return ti, nil
}
