// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// SCGI agent handlet passes requests to backend SCGI servers and cache responses.

package internal

func init() {
	RegisterHandlet("scgiAgent", func(name string, stage *Stage, app *App) Handlet {
		h := new(scgiAgent)
		h.onCreate(name, stage, app)
		return h
	})
}

// scgiAgent handlet
type scgiAgent struct {
	// Mixins
	Handlet_
	contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage   // current stage
	app     *App     // the app to which the agent belongs
	backend PBackend // *TCPSBackend or *UnixBackend
	cacher  Cacher   // the cache server which is used by this agent
	// States
	bufferClientContent bool // client content is buffered anyway?
	bufferServerContent bool // server content is buffered anyway?
}

func (h *scgiAgent) onCreate(name string, stage *Stage, app *App) {
	h.SetUp(name)
	h.stage = stage
	h.app = app
}
func (h *scgiAgent) OnShutdown() {
	h.app.SubDone()
}

func (h *scgiAgent) OnConfigure() {
	h.contentSaver_.onConfigure(h, TempDir()+"/scgi/"+h.name)
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if pBackend, ok := backend.(PBackend); ok {
				h.backend = pBackend
			} else {
				UseExitf("incorrect backend '%s' for scgiAgent\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for scgiAgent")
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
func (h *scgiAgent) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *scgiAgent) IsProxy() bool { return true }
func (h *scgiAgent) IsCache() bool { return h.cacher != nil }

func (h *scgiAgent) Handle(req Request, resp Response) (next bool) {
	// TODO: implementation, use PConn
	resp.Send("scgi")
	return
}

// scgiStream
type scgiStream struct {
	// TODO
}

// scgiRequest
type scgiRequest struct { // outgoing. needs building
	// TODO
}

// scgiResponse
type scgiResponse struct { // incoming. needs parsing
	// TODO
}

// SCGI protocol elements.
