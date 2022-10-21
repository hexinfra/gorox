// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The HTTP/3 outgate.

package internal

import (
	"time"
)

func init() {
	registerFixture(signHTTP3)
}

const signHTTP3 = "http3"

func createHTTP3(stage *Stage) *HTTP3Outgate {
	http3 := new(HTTP3Outgate)
	http3.init(stage)
	http3.setShell(http3)
	return http3
}

// HTTP3Outgate
type HTTP3Outgate struct {
	// Mixins
	httpOutgate_
	// States
}

func (f *HTTP3Outgate) init(stage *Stage) {
	f.httpOutgate_.init(signHTTP3, stage)
}

func (f *HTTP3Outgate) OnConfigure() {
	f.configure()
}
func (f *HTTP3Outgate) OnPrepare() {
	f.prepare()
}
func (f *HTTP3Outgate) OnShutdown() {
	f.shutdown()
}

func (f *HTTP3Outgate) run() { // blocking
	for {
		time.Sleep(time.Second)
	}
}

func (f *HTTP3Outgate) FetchConn(address string) (*H3Conn, error) {
	// TODO
	return nil, nil
}
func (f *HTTP3Outgate) StoreConn(conn *H3Conn) {
	// TODO
}
