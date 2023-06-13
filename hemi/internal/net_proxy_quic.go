// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC proxy implementation.

package internal

func init() {
	RegisterQUICFilter("quicProxy", func(name string, stage *Stage, mesher *QUICMesher) QUICFilter {
		f := new(quicProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// quicProxy passes QUIC connections to backend QUIC server.
type quicProxy struct {
	// Mixins
	QUICFilter_
	// Assocs
	stage   *Stage
	mesher  *QUICMesher
	backend *QUICBackend
	// States
}

func (f *quicProxy) onCreate(name string, stage *Stage, mesher *QUICMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *quicProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *quicProxy) OnConfigure() {
	// toBackend
	if v, ok := f.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := f.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if quicBackend, ok := backend.(*QUICBackend); ok {
				f.backend = quicBackend
			} else {
				UseExitf("incorrect backend '%s' for quicProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for quicProxy")
	}
}
func (f *quicProxy) OnPrepare() {
}

func (f *quicProxy) Deal(connection *QUICConnection, stream *QUICStream) (next bool) { // reverse only
	// TODO
	return
}
