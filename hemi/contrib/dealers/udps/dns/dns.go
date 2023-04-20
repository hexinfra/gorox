// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// DNS dealers can respond DNS requests.

package dns

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterUDPSDealer("dnsDealer", func(name string, stage *Stage, mesher *UDPSMesher) UDPSDealer {
		f := new(dnsDealer)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// dnsDealer
type dnsDealer struct {
	// Mixins
	UDPSDealer_
	// Assocs
	stage  *Stage
	mesher *UDPSMesher
	// States
}

func (f *dnsDealer) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *dnsDealer) OnShutdown() {
	f.mesher.SubDone()
}

func (f *dnsDealer) OnConfigure() {
	// TODO
}
func (f *dnsDealer) OnPrepare() {
	// TODO
}

func (f *dnsDealer) Process(conn *UDPSConn) (next bool) {
	// TODO
	return
}
