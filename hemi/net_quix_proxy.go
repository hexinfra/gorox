// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIX (QUIC over UDP/UDS) reverse proxy implementation.

package hemi

func init() {
	RegisterQUIXDealet("quixProxy", func(compName string, stage *Stage, router *QUIXRouter) QUIXDealet {
		d := new(quixProxy)
		d.onCreate(compName, stage, router)
		return d
	})
}

// quixProxy dealet passes QUIX connections to QUIX backends.
type quixProxy struct {
	// Parent
	QUIXDealet_
	// Assocs
	router  *QUIXRouter  // the router to which the dealet belongs
	backend *QUIXBackend // the backend to pass to
	// States
	QUIXProxyConfig // embeded
}

func (d *quixProxy) onCreate(compName string, stage *Stage, router *QUIXRouter) {
	d.QUIXDealet_.OnCreate(compName, stage)
	d.router = router
}
func (d *quixProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *quixProxy) OnConfigure() {
	// .toBackend
	if v, ok := d.Find("toBackend"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if backend := d.stage.Backend(compName); backend == nil {
				UseExitf("unknown backend: '%s'\n", compName)
			} else if quixBackend, ok := backend.(*QUIXBackend); ok {
				d.backend = quixBackend
			} else {
				UseExitf("incorrect backend '%s' for quixProxy\n", compName)
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

func (d *quixProxy) DealWith(conn *QUIXConn, stream *QUIXStream) (dealt bool) {
	QUIXReverseProxy(conn, stream, d.backend, &d.QUIXProxyConfig)
	return true
}

// QUIXProxyConfig
type QUIXProxyConfig struct {
	// TODO
}

// QUIXReverseProxy
func QUIXReverseProxy(servConn *QUIXConn, servStream *QUIXStream, backend *QUIXBackend, proxyConfig *QUIXProxyConfig) {
	// TODO
}
