// Copyright (c) 2020-2023 FengWei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// tcp for FreeBSD.
package system

var (
	IPPROTO_TCP = 6
	TCP_INFO    = 32
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
	State          TCPState
	Ca_state       uint8
	Retransmits    uint8
	Probes         uint8
	Backoff        uint8
	Options        uint8
	Pad_cgo_0      [2]byte
	Rto            uint32
	Ato            uint32
	Snd_mss        uint32
	Rcv_mss        uint32
	Unacked        uint32
	Sacked         uint32
	Lost           uint32
	Retrans        uint32
	Fackets        uint32
	Last_data_sent uint32
	Last_ack_sent  uint32
	Last_data_recv uint32
	Last_ack_recv  uint32
	Pmtu           uint32
	Rcv_ssthresh   uint32
	Rtt            uint32
	Rttvar         uint32
	Snd_ssthresh   uint32
	Snd_cwnd       uint32
	Advmss         uint32
	Reordering     uint32
	Rcv_rtt        uint32
	Rcv_space      uint32
	Total_retrans  uint32
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
		uintptr(TCP_INFO),
		uintptr(unsafe.Pointer(ti)),
		uintptr(unsafe.Pointer(&TCPInfoSize)),
		0,
	)

	if retCode != 0 {
		return nil, err
	}

	return ti, nil
}
