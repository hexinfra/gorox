// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// DNS filters can respond DNS requests.

package dns

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterUDPSFilter("dnsFilter", func(name string, stage *Stage, mesher *UDPSMesher) UDPSFilter {
		f := new(dnsFilter)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// dnsFilter
type dnsFilter struct {
	// Mixins
	UDPSFilter_
	// Assocs
	stage  *Stage
	mesher *UDPSMesher
	// States
}

func (f *dnsFilter) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *dnsFilter) OnShutdown() {
	f.mesher.SubDone()
}

func (f *dnsFilter) OnConfigure() {
}
func (f *dnsFilter) OnPrepare() {
}

func (f *dnsFilter) Deal(conn *UDPSConn) (next bool) {
	// TODO
	return
}
