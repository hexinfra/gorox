// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 outgate.

package internal

import (
	"crypto/tls"
	"net"
	"time"
)

func init() {
	registerFixture(signHTTP1Outgate)
}

const signHTTP1Outgate = "http1Outgate"

func createHTTP1Outgate(stage *Stage) *HTTP1Outgate {
	http1 := new(HTTP1Outgate)
	http1.onCreate(stage)
	http1.setShell(http1)
	return http1
}

// HTTP1Outgate
type HTTP1Outgate struct {
	// Mixins
	webOutgate_
	// States
	conns any // TODO
}

func (f *HTTP1Outgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHTTP1Outgate, stage)
}

func (f *HTTP1Outgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HTTP1Outgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HTTP1Outgate) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("http1Outgate done")
	}
	f.stage.SubDone()
}

func (f *HTTP1Outgate) DialTCP(address string) (*H1Conn, error) {
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
	return getH1Conn(connID, false, false, f, nil, netConn, rawConn), nil
}
func (f *HTTP1Outgate) DialTLS(address string, tlsConfig *tls.Config) (*H1Conn, error) {
	netConn, err := net.DialTimeout("tcp", address, f.dialTimeout)
	if err != nil {
		return nil, err
	}
	connID := f.nextConnID()
	tlsConn := tls.Client(netConn, tlsConfig)
	return getH1Conn(connID, false, true, f, nil, tlsConn, nil), nil
}
