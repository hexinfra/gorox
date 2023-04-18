// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS relay filter implementation.

package internal

func init() {
	RegisterUDPSFilter("udpsRelay", func(name string, stage *Stage, mesher *UDPSMesher) UDPSFilter {
		f := new(udpsRelay)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// udpsRelay relays UDP/DTLS connections to backend UDP/DTLS server.
type udpsRelay struct {
	// Mixins
	UDPSFilter_
	// Assocs
	stage   *Stage
	mesher  *UDPSMesher
	backend *UDPSBackend
	// States
}

func (f *udpsRelay) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *udpsRelay) OnShutdown() {
	f.mesher.SubDone()
}

func (f *udpsRelay) OnConfigure() {
	// toBackend
	if v, ok := f.Prop("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := f.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if udpsBackend, ok := backend.(*UDPSBackend); ok {
				f.backend = udpsBackend
			} else {
				UseExitf("incorrect backend '%s' for udpsRelay\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for udpsRelay")
	}
}
func (f *udpsRelay) OnPrepare() {
}

func (f *udpsRelay) Handle(conn *UDPSConn) (next bool) {
	// TODO
	return
}
