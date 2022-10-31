// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The TCPS outgate.

package internal

import (
	"crypto/tls"
	"net"
)

func init() {
	registerFixture(signTCPS)
}

const signTCPS = "tcps"

func createTCPS(stage *Stage) *TCPSOutgate {
	tcps := new(TCPSOutgate)
	tcps.init(stage)
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

func (f *TCPSOutgate) init(stage *Stage) {
	f.outgate_.init(signTCPS, stage)
}

func (f *TCPSOutgate) OnConfigure() {
	f.outgate_.onConfigure()
	// maxStreamsPerConn
	f.ConfigureInt32("maxStreamsPerConn", &f.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
}
func (f *TCPSOutgate) OnPrepare() {
	f.outgate_.onPrepare()
}
func (f *TCPSOutgate) OnShutdown() {
	f.outgate_.onShutdown()
}

func (f *TCPSOutgate) run() { // blocking
	// TODO
}

func (f *TCPSOutgate) Dial(address string, tlsMode bool) (*TConn, error) {
	netConn, err := net.DialTimeout("tcp", address, f.dialTimeout)
	if err != nil {
		return nil, err
	}
	connID := f.nextConnID()
	if tlsMode {
		tlsConn := tls.Client(netConn, nil)
		return getTConn(connID, f, nil, tlsConn, nil), nil
	} else {
		rawConn, err := netConn.(*net.TCPConn).SyscallConn()
		if err != nil {
			netConn.Close()
			return nil, err
		}
		return getTConn(connID, f, nil, netConn, rawConn), nil
	}
}
func (f *TCPSOutgate) FetchConn(address string, tlsMode bool) (*TConn, error) {
	// TODO
	return nil, nil
}
func (f *TCPSOutgate) StoreConn(conn *TConn) {
	// TODO
}
