// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// AJP proxy implementation.

package internal

import (
	"errors"
	"time"
)

func init() {
	RegisterHandlet("ajpProxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(ajpProxy)
		h.onCreate(name, stage, app)
		return h
	})
}

// ajpProxy handlet passes web requests to backend AJP servers and cache responses.
type ajpProxy struct {
	// Mixins
	Handlet_
	contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage       // current stage
	app     *App         // the app to which the proxy belongs
	backend *TCPSBackend // the ajp backend to pass to
	storer  Storer       // the storer which is used by this proxy
	// States
	bufferClientContent bool          // client content is buffered anyway?
	bufferServerContent bool          // server content is buffered anyway?
	sendTimeout         time.Duration // timeout to send the whole request
	recvTimeout         time.Duration // timeout to recv the whole response content
	maxContentSize      int64         // max response content size allowed
}

func (h *ajpProxy) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *ajpProxy) OnShutdown() {
	h.app.SubDone()
}

func (h *ajpProxy) OnConfigure() {
	h.contentSaver_.onConfigure(h, TempDir()+"/ajp/"+h.name)
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				h.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for ajpProxy, must be TCPSBackend\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for ajpProxy")
	}

	// withStorer
	if v, ok := h.Find("withStorer"); ok {
		if name, ok := v.String(); ok && name != "" {
			if storer := h.stage.Storer(name); storer == nil {
				UseExitf("unknown storer: '%s'\n", name)
			} else {
				h.storer = storer
			}
		} else {
			UseExitln("invalid withStorer")
		}
	}

	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)

	// sendTimeout
	h.ConfigureDuration("sendTimeout", &h.sendTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, 60*time.Second)

	// recvTimeout
	h.ConfigureDuration("recvTimeout", &h.recvTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, 60*time.Second)

	// maxContentSize
	h.ConfigureInt64("maxContentSize", &h.maxContentSize, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxContentSize has an invalid value")
	}, _1T)
}
func (h *ajpProxy) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *ajpProxy) IsProxy() bool { return true }
func (h *ajpProxy) IsCache() bool { return h.storer != nil }

func (h *ajpProxy) Handle(req Request, resp Response) (next bool) { // reverse only
	// TODO: implementation
	resp.Send("ajp")
	return
}
