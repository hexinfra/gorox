// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB agent handlet passes requests to backend HWEB servers and cache responses.

package internal

func init() {
	RegisterHandlet("hwebAgent", func(name string, stage *Stage, app *App) Handlet {
		h := new(hwebAgent)
		h.onCreate(name, stage, app)
		return h
	})
}

// hwebAgent
type hwebAgent struct {
	// Mixins
	Handlet_
	contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage       // current stage
	app     *App         // the app to which the agent belongs
	backend *hwebBackend // ...
	cacher  Cacher       // the cacher which is used by this agent
	// States
	bufferClientContent bool // client content is buffered anyway?
	bufferServerContent bool // server content is buffered anyway?
}

func (h *hwebAgent) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *hwebAgent) OnShutdown() {
	h.app.SubDone()
}

func (h *hwebAgent) OnConfigure() {
	h.contentSaver_.onConfigure(h, TempDir()+"/hweb/"+h.name)
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if hwebBackend, ok := backend.(*hwebBackend); ok {
				h.backend = hwebBackend
			} else {
				UseExitf("incorrect backend '%s' for hwebAgent\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for hwebAgent")
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
func (h *hwebAgent) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *hwebAgent) IsProxy() bool { return true }
func (h *hwebAgent) IsCache() bool { return h.cacher != nil }

func (h *hwebAgent) Handle(req Request, resp Response) (next bool) { // reverse only
	return
}
