// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 outgate.

package internal

import (
	"time"
)

func init() {
	registerFixture(signHTTP2Outgate)
}

const signHTTP2Outgate = "http2Outgate"

func createHTTP2Outgate(stage *Stage) *HTTP2Outgate {
	http2 := new(HTTP2Outgate)
	http2.onCreate(stage)
	http2.setShell(http2)
	return http2
}

// HTTP2Outgate
type HTTP2Outgate struct {
	// Mixins
	webOutgate_
	// States
}

func (f *HTTP2Outgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHTTP2Outgate, stage)
}

func (f *HTTP2Outgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HTTP2Outgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HTTP2Outgate) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("http2Outgate done")
	}
	f.stage.SubDone()
}

func (f *HTTP2Outgate) FetchConn(address string) (*H2Conn, error) {
	// TODO
	return nil, nil
}
func (f *HTTP2Outgate) StoreConn(conn *H2Conn) {
	// TODO
}
