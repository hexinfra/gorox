// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// DNS runners can respond DNS requests.

package dns

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterUDPSFilter("dnsFilter", func(name string, stage *Stage, router *UDPSRouter) UDPSFilter {
		f := new(dnsFilter)
		f.init(name, stage, router)
		return f
	})
}

// dnsFilter
type dnsFilter struct {
	// Mixins
	UDPSFilter_
	// Assocs
	stage  *Stage
	router *UDPSRouter
	// States
}

func (f *dnsFilter) init(name string, stage *Stage, router *UDPSRouter) {
	f.SetName(name)
	f.stage = stage
	f.router = router
}

func (f *dnsFilter) OnConfigure() {
}
func (f *dnsFilter) OnPrepare() {
}
func (f *dnsFilter) OnShutdown() {
}

func (f *dnsFilter) Process(conn *UDPSConn) (next bool) {
	return
}
