// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC (UDP/UDS) reverse proxy implementation.

package hemi

func init() {
	RegisterQUICDealet("quicProxy", func(name string, stage *Stage, router *QUICRouter) QUICDealet {
		d := new(quicProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// quicProxy passes QUIC connections to backend QUIC server.
type quicProxy struct {
	// Mixins
	QUICDealet_
	// Assocs
	stage   *Stage // current stage
	router  *QUICRouter
	backend *QUICBackend
	// States
}

func (d *quicProxy) onCreate(name string, stage *Stage, router *QUICRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *quicProxy) OnShutdown() {
	d.router.SubDone()
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

func (d *quicProxy) Deal(conn *QUICConn, stream *QUICStream) (dealt bool) {
	// TODO
	return true
}
