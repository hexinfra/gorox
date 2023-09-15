// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Net for Windows.

package system

import (
	"syscall"
	"unsafe"
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

var (
	//link: https://learn.microsoft.com/en-us/windows/win32/winsock/winsock-ioctls
	sioTcpInfo = uint32(syscall.IOC_INOUT | syscall.IOC_VENDOR | 39)
)

// TCPState  the possible states of a TCP connection.
// link: https://learn.microsoft.com/en-us/windows/win32/api/mstcpip/ne-mstcpip-tcpstate
type TCPState uint32

const (
	TCPStateClosed TCPState = iota
	TCPStateListen
	TCPStateSynSent
	TCPStateSynRcvd
	TCPStateEstablished
	TCPStateFinWait1
	TCPStateFinWait2
	TCPStateCloseWait
	TCPStateClosing
	TCPStateLastAck
	TCPStateTimeWait
)

// TCPInfo tcp_info_v0
// link: https://learn.microsoft.com/en-us/windows/win32/api/mstcpip/ns-mstcpip-tcp_info_v0
type TCPInfo struct {
	State TCPState
}

func (t *TCPInfo) IsEstablished() bool {
	return t.State == TCPStateEstablished
}

func (t *TCPInfo) CanWrite() bool {
	return t.IsEstablished() || t.State == TCPStateCloseWait
}

// GetTCPInfo  TCP statistics for a specified socket.
// link: https://learn.microsoft.com/en-us/windows/win32/winsock/sio-tcp-info
func GetTCPInfo(sockfd uintptr) (*TCPInfo, error) {
	ti := TCPInfo{}
	length := uint32(unsafe.Sizeof(ti))
	version := uint32(0)

	err := syscall.WSAIoctl(
		syscall.Handle(sockfd),
		sioTcpInfo,
		(*byte)(unsafe.Pointer(&version)),
		4,
		(*byte)(unsafe.Pointer(&ti)),
		length,
		&length,
		nil,
		uintptr(0),
	)

	return &ti, err
}
