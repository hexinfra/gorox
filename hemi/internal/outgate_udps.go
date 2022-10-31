// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The UDPS outgate.

package internal

func init() {
	registerFixture(signUDPS)
}

const signUDPS = "udps"

func createUDPS(stage *Stage) *UDPSOutgate {
	udps := new(UDPSOutgate)
	udps.init(stage)
	udps.setShell(udps)
	return udps
}

// UDPSOutgate component.
type UDPSOutgate struct {
	// Mixins
	outgate_
	// States
}

func (f *UDPSOutgate) init(stage *Stage) {
	f.outgate_.init(signUDPS, stage)
}

func (f *UDPSOutgate) OnConfigure() {
	f.outgate_.onConfigure()
}
func (f *UDPSOutgate) OnPrepare() {
	f.outgate_.onPrepare()
}
func (f *UDPSOutgate) OnShutdown() {
	f.outgate_.onShutdown()
}

func (f *UDPSOutgate) run() { // blocking
	// TODO
}

func (f *UDPSOutgate) Dial(address string, tlsMode bool) (*UConn, error) {
	// TODO
	return nil, nil
}
func (f *UDPSOutgate) FetchConn(address string, tlsMode bool) (*UConn, error) {
	// TODO
	return nil, nil
}
func (f *UDPSOutgate) StoreConn(conn *UConn) {
	// TODO
}
