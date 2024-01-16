// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS proxy implementation.

package internal

func init() {
	RegisterUDPSFilter("udpsProxy", func(name string, stage *Stage, mesher *UDPSMesher) UDPSFilter {
		f := new(udpsProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// udpsProxy passes UDP/DTLS links to backend UDP/DTLS server.
type udpsProxy struct {
	// Mixins
	UDPSFilter_
	// Assocs
	stage   *Stage
	mesher  *UDPSMesher
	backend *UDPSBackend
	// States
}

func (f *udpsProxy) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *udpsProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *udpsProxy) OnConfigure() {
	// toBackend
	if v, ok := f.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := f.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if udpsBackend, ok := backend.(*UDPSBackend); ok {
				f.backend = udpsBackend
			} else {
				UseExitf("incorrect backend '%s' for udpsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for udpsProxy")
	}
}
func (f *udpsProxy) OnPrepare() {
}

func (f *udpsProxy) Deal(link *UDPSLink) (next bool) { // reverse only
	// TODO
	return
}
