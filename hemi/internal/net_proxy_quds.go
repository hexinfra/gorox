// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUDS (QUIC over UUDS) proxy implementation.

package internal

func init() {
	RegisterQUICFilter("qudsProxy", func(name string, stage *Stage, mesher *QUICMesher) QUICFilter {
		f := new(qudsProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// qudsProxy passes QUIC connections to backend QUDS server.
type qudsProxy struct {
	// Mixins
	QUICFilter_
	// Assocs
	stage   *Stage
	mesher  *QUICMesher
	backend *QUDSBackend
	// States
}

func (f *qudsProxy) onCreate(name string, stage *Stage, mesher *QUICMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *qudsProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *qudsProxy) OnConfigure() {
	// toBackend
	if v, ok := f.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := f.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if qudsBackend, ok := backend.(*QUDSBackend); ok {
				f.backend = qudsBackend
			} else {
				UseExitf("incorrect backend '%s' for qudsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for qudsProxy")
	}
}
func (f *qudsProxy) OnPrepare() {
}

func (f *qudsProxy) Deal(connection *QUICConnection, stream *QUICStream) (next bool) { // reverse only
	// TODO
	return
}
