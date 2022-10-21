// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The Unix outgate.

package internal

func init() {
	registerFixture(signUnix)
}

const signUnix = "unix"

func createUnix(stage *Stage) *UnixOutgate {
	unix := new(UnixOutgate)
	unix.init(stage)
	unix.setShell(unix)
	return unix
}

// UnixOutgate component.
type UnixOutgate struct {
	// Mixins
	outgate_
	streamHolder_
	// States
}

func (f *UnixOutgate) init(stage *Stage) {
	f.outgate_.init(signUnix, stage)
}

func (f *UnixOutgate) OnConfigure() {
	f.configure()
	// maxStreamsPerConn
	f.ConfigureInt32("maxStreamsPerConn", &f.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
}
func (f *UnixOutgate) OnPrepare() {
	f.prepare()
}
func (f *UnixOutgate) OnShutdown() {
	f.shutdown()
}

func (f *UnixOutgate) run() { // blocking
	// TODO
}

func (f *UnixOutgate) Dial(address string) (*XConn, error) {
	// TODO
	return nil, nil
}
func (f *UnixOutgate) FetchConn(address string) (*XConn, error) {
	// TODO
	return nil, nil
}
func (f *UnixOutgate) StoreConn(conn *XConn) {
	// TODO
}
