// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The QUIC outgate.

package internal

func init() {
	registerFixture(signQUIC)
}

const signQUIC = "quic"

func createQUIC(stage *Stage) *QUICOutgate {
	quic := new(QUICOutgate)
	quic.init(stage)
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

func (f *QUICOutgate) init(stage *Stage) {
	f.outgate_.init(signQUIC, stage)
}

func (f *QUICOutgate) OnConfigure() {
	f.outgate_.onConfigure()
	// maxStreamsPerConn
	f.ConfigureInt32("maxStreamsPerConn", &f.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
}
func (f *QUICOutgate) OnPrepare() {
	f.outgate_.onPrepare()
}
func (f *QUICOutgate) OnShutdown() {
	f.outgate_.onShutdown()
}

func (f *QUICOutgate) run() { // blocking
	// TODO
}

func (f *QUICOutgate) Dial(address string) (*QConn, error) {
	// TODO
	return nil, nil
}
func (f *QUICOutgate) FetchConn(address string) {
	// TODO
}
func (f *QUICOutgate) StoreConn(conn *QConn) {
	// TODO
}
