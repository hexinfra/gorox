// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Net for FreeBSD.

package system

import (
	"syscall"
	"unsafe"
)

func SetDeferAccept(rawConn syscall.RawConn) (err error) {
	return
}

func SetReusePort(rawConn syscall.RawConn) (err error) {
	// A maximum of	256 processes can share	one socket.
	const SO_REUSEPORT_LB = 0x10000 // for amd64, arm64, riscv64
	rawConn.Control(func(fd uintptr) {
		err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, SO_REUSEPORT_LB, 1)
	})
	return
}

func SetBuffered(rawConn syscall.RawConn, buffered bool) {
	// TODO: tcp nopush
}

var (
	IPPROTO_TCP = 6
	TCP_INFO    = 32
	tcpInfoSize = unsafe.Sizeof(TCPInfo{})
)

// TCPState TCP FSM state
// link: /usr/include/netinet/tcp_fsm.h
type TCPState uint8

const (
	TCPStateClosed TCPState = iota
	TCPStateListen
	TCPStateSynSent
	TCPStateSynRcvd
	TCPStateEstablished
	TCPStateCloseWait
	TCPStateFinWait1
	TCPStateClosing
	TCPStateLastAck
	TCPStateFinWait2
	TCPStateTimeWait
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
		uintptr(IPPROTO_TCP),
		uintptr(TCP_INFO),
		uintptr(unsafe.Pointer(ti)),
		uintptr(unsafe.Pointer(&tcpInfoSize)),
		0,
	)

	if retCode != 0 {
		return nil, err
	}

	return ti, nil
}
