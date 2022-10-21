// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The HTTP/1 outgate.

package internal

import (
	"crypto/tls"
	"net"
	"time"
)

func init() {
	registerFixture(signHTTP1)
}

const signHTTP1 = "http1"

func createHTTP1(stage *Stage) *HTTP1Outgate {
	http1 := new(HTTP1Outgate)
	http1.init(stage)
	http1.setShell(http1)
	return http1
}

// HTTP1Outgate
type HTTP1Outgate struct {
	// Mixins
	httpOutgate_
	// States
	conns any // TODO
}

func (f *HTTP1Outgate) init(stage *Stage) {
	f.httpOutgate_.init(signHTTP1, stage)
}

func (f *HTTP1Outgate) OnConfigure() {
	f.configure()
}
func (f *HTTP1Outgate) OnPrepare() {
	f.prepare()
}
func (f *HTTP1Outgate) OnShutdown() {
	f.shutdown()
}

func (f *HTTP1Outgate) run() { // blocking
	for {
		// maintain?
		time.Sleep(time.Second)
	}
}

func (f *HTTP1Outgate) FetchConn(address string, tlsMode bool) (*H1Conn, error) {
	// TODO: takeConn
	netConn, err := net.DialTimeout("tcp", address, f.dialTimeout)
	if err != nil {
		return nil, err
	}
	connID := f.nextConnID()
	if tlsMode {
		tlsConn := tls.Client(netConn, f.tlsConfig) // TODO: tlsConfig
		return getH1Conn(connID, f, nil, tlsConn, nil), nil
	} else {
		rawConn, err := netConn.(*net.TCPConn).SyscallConn()
		if err != nil {
			netConn.Close()
			return nil, err
		}
		return getH1Conn(connID, f, nil, netConn, rawConn), nil
	}
}
func (f *HTTP1Outgate) StoreConn(conn *H1Conn) {
	if conn.isBroken() || !conn.isAlive() {
		conn.closeConn()
		putH1Conn(conn)
	} else {
		// TODO: pushConn
	}
}
