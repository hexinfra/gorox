// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC relay filter implementation.

package internal

func init() {
	RegisterQUICFilter("quicRelay", func(name string, stage *Stage, mesher *QUICMesher) QUICFilter {
		f := new(quicRelay)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// quicRelay relays QUIC connections to backend QUIC server.
type quicRelay struct {
	// Mixins
	QUICFilter_
	// Assocs
	stage   *Stage
	mesher  *QUICMesher
	backend *QUICBackend
	// States
}

func (f *quicRelay) onCreate(name string, stage *Stage, mesher *QUICMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *quicRelay) OnShutdown() {
	f.mesher.SubDone()
}

func (f *quicRelay) OnConfigure() {
	// toBackend
	if v, ok := f.Prop("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := f.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if quicBackend, ok := backend.(*QUICBackend); ok {
				f.backend = quicBackend
			} else {
				UseExitf("incorrect backend '%s' for quicRelay\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for quicRelay")
	}
}
func (f *quicRelay) OnPrepare() {
}

func (f *quicRelay) Handle(conn *QUICConn, stream *QUICStream) (next bool) {
	// TODO
	return
}
