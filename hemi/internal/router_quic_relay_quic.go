// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC relay implementation.

package internal

func init() {
	RegisterQUICDealer("quicRelay", func(name string, stage *Stage, router *QUICRouter) QUICDealer {
		d := new(quicRelay)
		d.onCreate(name, stage, router)
		return d
	})
}

// quicRelay passes QUIC connections to backend QUIC server.
type quicRelay struct {
	// Mixins
	QUICDealer_
	// Assocs
	stage   *Stage
	router  *QUICRouter
	backend *QUICBackend
	// States
}

func (d *quicRelay) onCreate(name string, stage *Stage, router *QUICRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *quicRelay) OnShutdown() {
	d.router.SubDone()
}

func (d *quicRelay) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if quicBackend, ok := backend.(*QUICBackend); ok {
				d.backend = quicBackend
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
func (d *quicRelay) OnPrepare() {
}

func (d *quicRelay) Deal(conn *QUICConn, stream *QUICStream) (next bool) { // reverse only
	// TODO
	return
}
