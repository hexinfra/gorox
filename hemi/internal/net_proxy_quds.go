// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUDS (QUIC over UUDS) proxy implementation.

package internal

func init() {
	RegisterQUICDealet("qudsProxy", func(name string, stage *Stage, mesher *QUICMesher) QUICDealet {
		d := new(qudsProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// qudsProxy passes QUIC connections to backend QUDS server.
type qudsProxy struct {
	// Mixins
	QUICDealet_
	// Assocs
	stage   *Stage
	mesher  *QUICMesher
	backend *QUDSBackend
	// States
}

func (d *qudsProxy) onCreate(name string, stage *Stage, mesher *QUICMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *qudsProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *qudsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if qudsBackend, ok := backend.(*QUDSBackend); ok {
				d.backend = qudsBackend
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
func (d *qudsProxy) OnPrepare() {
}

func (d *qudsProxy) OnSetup(connection *QUICConnection) (next bool) {
	// TODO
	return
}
func (d *qudsProxy) Deal(connection *QUICConnection, stream *QUICStream) (next bool) { // reverse only
	// TODO
	return
}
