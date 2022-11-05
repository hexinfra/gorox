// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UWSGI proxy handlers pass requests to backend uWSGI servers and cache responses.

package uwsgi

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandler("uwsgiProxy", func(name string, stage *Stage, app *App) Handler {
		h := new(uwsgiProxy)
		h.init(name, stage, app)
		return h
	})
}

// uwsgiProxy handler
type uwsgiProxy struct {
	// Mixins
	Handler_
	// Assocs
	stage   *Stage
	app     *App
	backend PBackend // *TCPSBackend or *UnixBackend
	cacher  Cacher
	// States
	bufferClientContent bool // ...
	bufferServerContent bool // server content is buffered anyway?
}

func (h *uwsgiProxy) init(name string, stage *Stage, app *App) {
	h.Handler_.Init(name, h)
	h.stage = stage
	h.app = app
}

func (h *uwsgiProxy) OnConfigure() {
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if pBackend, ok := backend.(PBackend); ok {
				h.backend = pBackend
			} else {
				UseExitf("incorrect backend '%s' for uwsgiProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for uwsgiProxy")
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
func (h *uwsgiProxy) OnPrepare() {
}
func (h *uwsgiProxy) OnShutdown() {
}

func (h *uwsgiProxy) IsProxy() bool { return true }
func (h *uwsgiProxy) IsCache() bool { return h.cacher != nil }

func (h *uwsgiProxy) Handle(req Request, resp Response) (next bool) {
	// TODO: implementation, use PConn
	resp.Send("uwsgi")
	return
}
