// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The QUIC outgate.

package internal

import (
	"time"
)

func init() {
	registerFixture(signQUICOutgate)
}

const signQUICOutgate = "quicOutgate"

func createQUICOutgate(stage *Stage) *QUICOutgate {
	quic := new(QUICOutgate)
	quic.onCreate(stage)
	quic.setShell(quic)
	return quic
}

// QUICOutgate component.
type QUICOutgate struct {
	// Mixins
	outgate_
	streamHolder_
	// States
}

func (f *QUICOutgate) onCreate(stage *Stage) {
	f.outgate_.onCreate(signQUICOutgate, stage)
}

func (f *QUICOutgate) OnConfigure() {
	f.outgate_.onConfigure()
	f.streamHolder_.onConfigure(f, 1000)
}
func (f *QUICOutgate) OnPrepare() {
	f.outgate_.onPrepare()
	f.streamHolder_.onPrepare(f)
}

func (f *QUICOutgate) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("quicOutgate done")
	}
	f.stage.SubDone()
}

func (f *QUICOutgate) Dial(address string) (*QConn, error) {
	// TODO
	return nil, nil
}
func (f *QUICOutgate) FetchConn(address string) {
	// TODO
}
func (f *QUICOutgate) StoreConn(qConn *QConn) {
	// TODO
}
