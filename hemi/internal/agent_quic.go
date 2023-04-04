// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC agent dealet implementation.

package internal

func init() {
	RegisterQUICDealet("quicAgent", func(name string, stage *Stage, mesher *QUICMesher) QUICDealet {
		d := new(quicAgent)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// quicAgent relays QUIC connections to another QUIC server.
type quicAgent struct {
	// Mixins
	QUICDealet_
	// Assocs
	stage   *Stage
	backend *QUICBackend
	mesher  *QUICMesher
	// States
}

func (d *quicAgent) onCreate(name string, stage *Stage, mesher *QUICMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *quicAgent) OnShutdown() {
	d.mesher.SubDone()
}

func (d *quicAgent) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if quicBackend, ok := backend.(*QUICBackend); ok {
				d.backend = quicBackend
			} else {
				UseExitf("incorrect backend '%s' for quicAgent\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for quicAgent")
	}
}
func (d *quicAgent) OnPrepare() {
}

func (d *quicAgent) Deal(conn *QUICConn, stream *QUICStream) (next bool) {
	// TODO
	return
}
