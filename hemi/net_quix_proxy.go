// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIX (UDP/UDS) reverse proxy.

package hemi

func init() {
	RegisterQUIXDealet("quixProxy", func(name string, stage *Stage, router *QUIXRouter) QUIXDealet {
		d := new(quixProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// quixProxy passes QUIX connections to QUIX backends.
type quixProxy struct {
	// Parent
	QUIXDealet_
	// Assocs
	stage   *Stage // current stage
	router  *QUIXRouter
	backend *QUIXBackend // the backend to pass to
	// States
}

func (d *quixProxy) onCreate(name string, stage *Stage, router *QUIXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *quixProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *quixProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if quixBackend, ok := backend.(*QUIXBackend); ok {
				d.backend = quixBackend
			} else {
				UseExitf("incorrect backend '%s' for quixProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for quixProxy")
	}
}
func (d *quixProxy) OnPrepare() {
}

func (d *quixProxy) Deal(conn *QUIXConn, stream *QUIXStream) (dealt bool) {
	// TODO
	return true
}
