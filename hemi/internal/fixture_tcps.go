// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The TCPS outgate.

package internal

import (
	"crypto/tls"
	"net"
	"time"
)

func init() {
	registerFixture(signTCPSOutgate)
}

const signTCPSOutgate = "tcpsOutgate"

func createTCPSOutgate(stage *Stage) *TCPSOutgate {
	tcps := new(TCPSOutgate)
	tcps.onCreate(stage)
	tcps.setShell(tcps)
	return tcps
}

// TCPSOutgate component.
type TCPSOutgate struct {
	// Mixins
	outgate_
	streamHolder_
	// States
}

func (f *TCPSOutgate) onCreate(stage *Stage) {
	f.outgate_.onCreate(signTCPSOutgate, stage)
}

func (f *TCPSOutgate) OnConfigure() {
	f.outgate_.onConfigure()
	f.streamHolder_.onConfigure(f, 1000)
}
func (f *TCPSOutgate) OnPrepare() {
	f.outgate_.onPrepare()
	f.streamHolder_.onPrepare(f)
}

func (f *TCPSOutgate) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("tcpsOutgate done")
	}
	f.stage.SubDone()
}

func (f *TCPSOutgate) DialTCP(address string) (*TConn, error) {
	netConn, err := net.DialTimeout("tcp", address, f.dialTimeout)
	if err != nil {
		return nil, err
	}
	connID := f.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, sockTypeNET, false, f, nil, netConn, rawConn), nil
}
func (f *TCPSOutgate) DialTLS(address string, tlsConfig *tls.Config) (*TConn, error) {
	netConn, err := net.DialTimeout("tcp", address, f.dialTimeout)
	if err != nil {
		return nil, err
	}
	connID := f.nextConnID()
	tlsConn := tls.Client(netConn, tlsConfig)
	return getTConn(connID, sockTypeNET, true, f, nil, tlsConn, nil), nil
}
func (f *TCPSOutgate) DialUDS(address string) (*TConn, error) {
	// TODO
	return nil, nil
}
