// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UWSGI proxy handlet passes requests to backend uWSGI servers and cache responses.

// UWSGI is mainly for Python applications. See: https://uwsgi-docs.readthedocs.io/en/latest/Protocol.html
// UWSGI 1.9.13 seems to have unsized content support: https://uwsgi-docs.readthedocs.io/en/latest/Chunked.html

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
	contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage       // current stage
	app     *App         // the app to which the proxy belongs
	backend *TCPSBackend // the uwsgi backend to pass to
	cacher  Cacher       // the cacher which is used by this proxy
	// States
	bufferClientContent bool // client content is buffered anyway?
	bufferServerContent bool // server content is buffered anyway?
}

func (h *uwsgiProxy) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *uwsgiProxy) OnShutdown() {
	h.app.SubDone()
}

func (h *uwsgiProxy) OnConfigure() {
	h.contentSaver_.onConfigure(h, TempDir()+"/uwsgi/"+h.name)
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				h.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for uwsgiProxy, must be TCPSBackend\n", name)
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

	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
}
func (h *uwsgiProxy) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *uwsgiProxy) IsProxy() bool { return true }
func (h *uwsgiProxy) IsCache() bool { return h.cacher != nil }

func (h *uwsgiProxy) Handle(req Request, resp Response) (next bool) { // reverse only
	// TODO: implementation
	resp.Send("uwsgi")
	return
}

// uwsgiStream
type uwsgiStream struct {
	// TODO
}

// uwsgiRequest
type uwsgiRequest struct { // outgoing. needs building
	// TODO
}

// uwsgiResponse
type uwsgiResponse struct { // incoming. needs parsing
	// TODO
}

//////////////////////////////////////// UWSGI protocol elements ////////////////////////////////////////