// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UWSGI proxy handlets pass requests to backend uWSGI servers and cache responses.

// UWSGI is mainly for Python applications.

// UWSGI doesn't allow chunked content in HTTP, so we must buffer content.
// Until the whole content is buffered, we treat it as counted instead of chunked.

// UWSGI 1.9.13 seems to have solved this problem:
// https://uwsgi-docs.readthedocs.io/en/latest/Chunked.html

package internal

func init() {
	RegisterHandlet("uwsgiProxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(uwsgiProxy)
		h.onCreate(name, stage, app)
		return h
	})
}

// uwsgiProxy handlet
type uwsgiProxy struct {
	// Mixins
	Handlet_
	// Assocs
	stage   *Stage
	app     *App
	backend PBackend // *TCPSBackend or *UnixBackend
	cacher  Cacher
	// States
	bufferClientContent bool // ...
	bufferServerContent bool // server content is buffered anyway?
}

func (h *uwsgiProxy) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}
func (h *uwsgiProxy) OnShutdown() {
	h.app.SubDone()
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

func (h *uwsgiProxy) IsProxy() bool { return true }
func (h *uwsgiProxy) IsCache() bool { return h.cacher != nil }

func (h *uwsgiProxy) Handle(req Request, resp Response) (next bool) {
	// TODO: implementation, use PConn
	resp.Send("uwsgi")
	return
}
