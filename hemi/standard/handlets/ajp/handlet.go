// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// AJP proxy handlets pass requests to backend AJP servers and cache responses.

package ajp

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("ajpProxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(ajpProxy)
		h.onCreate(name, stage, app)
		return h
	})
}

// ajpProxy handlet
type ajpProxy struct {
	// Mixins
	Handlet_
	// Assocs
	stage   *Stage
	app     *App
	backend *TCPSBackend
	cacher  Cacher
	// States
	bufferClientContent bool // ...
	bufferServerContent bool // server content is buffered anyway?
}

func (h *ajpProxy) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}
func (h *ajpProxy) OnShutdown() {
	h.app.SubDone()
}

func (h *ajpProxy) OnConfigure() {
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				h.backend = backend.(*TCPSBackend)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for ajpProxy")
	}
	// withCacher
	if v, ok := h.Find("withCacher"); ok {
		if name, ok := v.String(); ok && name != "" {
			if cacher := h.stage.Cacher(name); cacher == nil {
				UseExitf("unknown cacher: '%s'\n", name)
			} else {
				h.cacher = cacher
			}
		} else {
			UseExitln("invalid withCacher")
		}
	}
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
}
func (h *ajpProxy) OnPrepare() {
}

func (h *ajpProxy) IsProxy() bool { return true }
func (h *ajpProxy) IsCache() bool { return h.cacher != nil }

func (h *ajpProxy) Handle(req Request, resp Response) (next bool) {
	// TODO: implementation, use *TConn
	resp.Send("ajp")
	return
}
