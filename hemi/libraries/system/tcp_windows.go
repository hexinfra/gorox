// Copyright (c) 2020-2023 FengWei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// tcp for Windows.
package system

import (
	"syscall"
	"unsafe"
)

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
	TCPStateMax
)

// TCPInfo tcp_info_v0
// link: https://learn.microsoft.com/en-us/windows/win32/api/mstcpip/ns-mstcpip-tcp_info_v0
type TCPInfo struct {
	State               TCPState
	Mss                 uint
	ConnectionTimeMs    uint
	TimestampsEnabled   uint8 // type boolean
	RttUs               uint
	MinRttUs            uint
	BytesInFlight       uint
	Cwnd                uint
	SndWnd              uint
	RcvWnd              uint
	RcvBuf              uint
	BytesOut            uint64
	BytesIn             uint64
	BytesReordered      uint
	BytesRetrans        uint
	FastRetrans         uint
	DupAcksIn           uint
	TimeoutEpisodesuint uint
	SynRetrans          uint8
}

func (t *TCPInfo) IsEstablished() bool {
	return t.State == TCPStateEstablished
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
