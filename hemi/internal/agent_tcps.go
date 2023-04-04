// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS agent dealet implementation.

package internal

func init() {
	RegisterTCPSDealet("tcpsAgent", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(tcpsAgent)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// tcpsAgent relays TCP/TLS connections to another TCP/TLS server.
type tcpsAgent struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage   *Stage
	backend *TCPSBackend
	mesher  *TCPSMesher
	// States
}

func (d *tcpsAgent) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *tcpsAgent) OnShutdown() {
	d.mesher.SubDone()
}

func (d *tcpsAgent) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				d.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for tcpsAgent\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for tcpsAgent")
	}
}
func (d *tcpsAgent) OnPrepare() {
}

func (d *tcpsAgent) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return
}
