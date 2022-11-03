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
	RegisterUDPSRunner("dnsRunner", func(name string, stage *Stage, router *UDPSRouter) UDPSRunner {
		r := new(dnsRunner)
		r.init(name, stage, router)
		return r
	})
}

// dnsRunner
type dnsRunner struct {
	// Mixins
	UDPSRunner_
	// Assocs
	stage  *Stage
	router *UDPSRouter
	// States
}

func (r *dnsRunner) init(name string, stage *Stage, router *UDPSRouter) {
	r.SetName(name)
	r.stage = stage
	r.router = router
}

func (r *dnsRunner) OnConfigure() {
}
func (r *dnsRunner) OnPrepare() {
}
func (r *dnsRunner) OnShutdown() {
}

func (r *dnsRunner) Process(conn *UDPSConn) (next bool) {
	return
}
