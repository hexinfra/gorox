// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UWSGI agent handlets pass requests to backend uWSGI servers and cache responses.

// UWSGI is mainly for Python applications.

// UWSGI doesn't allow chunked content in HTTP, so we must buffer content.
// Until the whole content is buffered, we treat it as counted instead of chunked.

// UWSGI 1.9.13 seems to have solved this problem:
// https://uwsgi-docs.readthedocs.io/en/latest/Chunked.html

package internal

func init() {
	RegisterHandlet("uwsgiAgent", func(name string, stage *Stage, app *App) Handlet {
		h := new(uwsgiAgent)
		h.onCreate(name, stage, app)
		return h
	})
}

// uwsgiAgent handlet
type uwsgiAgent struct {
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

func (h *uwsgiAgent) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}
func (h *uwsgiAgent) OnShutdown() {
	h.app.SubDone()
}

func (h *uwsgiAgent) OnConfigure() {
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if pBackend, ok := backend.(PBackend); ok {
				h.backend = pBackend
			} else {
				UseExitf("incorrect backend '%s' for uwsgiAgent\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for uwsgiAgent")
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
func (h *uwsgiAgent) OnPrepare() {
}

func (h *uwsgiAgent) IsProxy() bool { return true }
func (h *uwsgiAgent) IsCache() bool { return h.cacher != nil }

func (h *uwsgiAgent) Handle(req Request, resp Response) (next bool) {
	// TODO: implementation, use PConn
	resp.Send("uwsgi")
	return
}

// uwsgiStream
type uwsgiStream struct {
}

// uwsgiRequest
type uwsgiRequest struct { // outgoing. needs building
}

// uwsgiResponse
type uwsgiResponse struct { // incoming. needs parsing
}

// UWSGI protocol elements.
