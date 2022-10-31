// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The HTTP/2 outgate.

package internal

import (
	"time"
)

func init() {
	registerFixture(signHTTP2)
}

const signHTTP2 = "http2"

func createHTTP2(stage *Stage) *HTTP2Outgate {
	http2 := new(HTTP2Outgate)
	http2.init(stage)
	http2.setShell(http2)
	return http2
}

// HTTP2Outgate
type HTTP2Outgate struct {
	// Mixins
	httpOutgate_
	// States
}

func (f *HTTP2Outgate) init(stage *Stage) {
	f.httpOutgate_.init(signHTTP2, stage)
}

func (f *HTTP2Outgate) OnConfigure() {
	f.httpOutgate_.onConfigure()
}
func (f *HTTP2Outgate) OnPrepare() {
	f.httpOutgate_.onPrepare()
}
func (f *HTTP2Outgate) OnShutdown() {
	f.httpOutgate_.onShutdown()
}

func (f *HTTP2Outgate) run() { // blocking
	for {
		time.Sleep(time.Second)
	}
}

func (f *HTTP2Outgate) FetchConn(address string, tlsMode bool) (*H2Conn, error) {
	// TODO
	return nil, nil
}
func (f *HTTP2Outgate) StoreConn(conn *H2Conn) {
	// TODO
}
