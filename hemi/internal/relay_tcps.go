// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS relay filter implementation.

package internal

func init() {
	RegisterTCPSFilter("tcpsRelay", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(tcpsRelay)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// tcpsRelay relays TCP/TLS connections to backend TCP/TLS server.
type tcpsRelay struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage   *Stage
	mesher  *TCPSMesher
	backend *TCPSBackend
	// States
}

func (f *tcpsRelay) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *tcpsRelay) OnShutdown() {
	f.mesher.SubDone()
}

func (f *tcpsRelay) OnConfigure() {
	// toBackend
	if v, ok := f.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := f.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				f.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for tcpsRelay\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for tcpsRelay")
	}
}
func (f *tcpsRelay) OnPrepare() {
}

func (f *tcpsRelay) Handle(conn *TCPSConn) (next bool) {
	// TODO
	return
}
