// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The UDPS outgate.

package internal

import (
	"time"
)

func init() {
	registerFixture(signUDPSOutgate)
}

const signUDPSOutgate = "udpsOutgate"

func createUDPSOutgate(stage *Stage) *UDPSOutgate {
	udps := new(UDPSOutgate)
	udps.onCreate(stage)
	udps.setShell(udps)
	return udps
}

// UDPSOutgate component.
type UDPSOutgate struct {
	// Mixins
	outgate_
	// States
}

func (f *UDPSOutgate) onCreate(stage *Stage) {
	f.outgate_.onCreate(signUDPSOutgate, stage)
}

func (f *UDPSOutgate) OnConfigure() {
	f.outgate_.onConfigure()
}
func (f *UDPSOutgate) OnPrepare() {
	f.outgate_.onConfigure()
}

func (f *UDPSOutgate) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("udpsOutgate done")
	}
	f.stage.SubDone()
}

func (f *UDPSOutgate) DialUDP(address string) (*UConn, error) {
	// TODO
	return nil, nil
}
func (f *UDPSOutgate) DialUDS(address string) (*UConn, error) {
	// TODO
	return nil, nil
}
