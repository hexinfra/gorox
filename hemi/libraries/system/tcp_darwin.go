// Copyright (c) 2020-2023 FengWei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// tcp for macOS.

package system

import (
	"syscall"
	"unsafe"
)

var (
	IPPROTO_TCP         = 0x6
	TCP_CONNECTION_INFO = 0x106

	TCPInfoSize = unsafe.Sizeof(TCPInfo{})
)

// TCPState TCP FSM state
// link: usr/include/netinet/tcp_fsm.h
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
	TCPStateMax
)

// TCPInfo TCP statistics for a specified socket.
// link: usr/include/netinet/tcp.h
type TCPInfo struct {
	State      TCPState
	Snd_wscale uint8
	Rcv_wscale uint8
	//   u_int8_t        __pad1;
	Options             uint32
	Flags               uint32
	Rto                 uint32
	Maxseg              uint32
	Snd_ssthresh        uint32
	Snd_cwnd            uint32
	Snd_wnd             uint32
	Snd_sbbytes         uint32
	Rcv_wnd             uint32
	Rttcur              uint32
	Srtt                uint32
	Rttvar              uint32
	Txpackets           uint64
	Txbytes             uint64
	Txretransmitbytes   uint64
	Rxpackets           uint64
	Rxbytes             uint64
	Rxoutoforderbytes   uint64
	Txretransmitpackets uint64
}

func (t *TCPInfo) IsEstablished() bool {
	return t.State == TCPStateEstablished
}

func GetsockoptTCPInfo(sockfd uintptr) (*TCPInfo, error) {
	ti := &TCPInfo{}
	retCode, _, err := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		uintptr(sockfd),
		uintptr(IPPROTO_TCP),
		uintptr(TCP_CONNECTION_INFO),
		uintptr(unsafe.Pointer(ti)),
		uintptr(unsafe.Pointer(&TCPInfoSize)),
		0,
	)

	if retCode != 0 {
		return nil, err
	}

	return ti, nil
}
