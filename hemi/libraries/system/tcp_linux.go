// Copyright (c) 2020-2023 Feng Wei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP for Linux.

package system

import (
	"syscall"
	"unsafe"
)

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
