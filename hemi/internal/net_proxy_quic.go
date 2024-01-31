// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC proxy implementation.

package internal

func init() {
	RegisterQUICDealet("quicProxy", func(name string, stage *Stage, mesher *QUICMesher) QUICDealet {
		d := new(quicProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// quicProxy passes QUIC connections to backend QUIC server.
type quicProxy struct {
	// Mixins
	QUICDealet_
	// Assocs
	stage   *Stage
	mesher  *QUICMesher
	backend *QUICBackend
	// States
}

func (d *quicProxy) onCreate(name string, stage *Stage, mesher *QUICMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *quicProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *quicProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if quicBackend, ok := backend.(*QUICBackend); ok {
				d.backend = quicBackend
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
func (d *quicProxy) OnPrepare() {
}

func (d *quicProxy) Deal(connection *QUICConnection, stream *QUICStream) (dealt bool) { // reverse only
	// TODO
	return true
}
