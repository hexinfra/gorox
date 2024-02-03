// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 outgate.

package internal

import (
	"time"
)

func init() {
	registerFixture(signHTTP3Outgate)
}

const signHTTP3Outgate = "http3Outgate"

func createHTTP3Outgate(stage *Stage) *HTTP3Outgate {
	http3 := new(HTTP3Outgate)
	http3.onCreate(stage)
	http3.setShell(http3)
	return http3
}

// HTTP3Outgate
type HTTP3Outgate struct {
	// Mixins
	webOutgate_
	// States
}

func (f *HTTP3Outgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHTTP3Outgate, stage)
}

func (f *HTTP3Outgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HTTP3Outgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HTTP3Outgate) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("http3Outgate done")
	}
	f.stage.SubDone()
}

func (f *HTTP3Outgate) FetchConn(address string, tlsMode bool) (*H3Conn, error) {
	// TODO
	return nil, nil
}
func (f *HTTP3Outgate) StoreConn(conn *H3Conn) {
	// TODO
}
