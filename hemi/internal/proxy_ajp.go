// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// AJP proxy handlet passes requests to backend AJP servers and cache responses.

// See: https://tomcat.apache.org/connectors-doc/ajp/ajpv13a.html

// I'm not sure whether AJP supports HTTP unsized content.
// If it doesn't, we have to buffer the request content.

// It seems AJP does support unsized request content, see below:

// Get Body Chunk

// The container asks for more data from the request (If the body was too large to fit in the first packet sent over or when the request is chuncked). The server will send a body packet back with an amount of data which is the minimum of the request_length, the maximum send body size (8186 (8 Kbytes - 6)), and the number of bytes actually left to send from the request body.
// If there is no more data in the body (i.e. the servlet container is trying to read past the end of the body), the server will send back an "empty" packet, which is a body packet with a payload length of 0. (0x12,0x34,0x00,0x00)

package internal

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
	contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage       // current stage
	app     *App         // the app to which the proxy belongs
	backend *TCPSBackend // the ajp backend to pass to
	cacher  Cacher       // the cacher which is used by this proxy
	// States
	bufferClientContent bool // client content is buffered anyway?
	bufferServerContent bool // server content is buffered anyway?
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
func (h *ajpProxy) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *ajpProxy) IsProxy() bool { return true }
func (h *ajpProxy) IsCache() bool { return h.cacher != nil }

func (h *ajpProxy) Handle(req Request, resp Response) (next bool) { // reverse only
	// TODO: implementation
	resp.Send("ajp")
	return
}

// ajpStream
type ajpStream struct {
	// TODO
}

// ajpRequest
type ajpRequest struct { // outgoing. needs building
	// TODO
}

// ajpResponse
type ajpResponse struct { // incoming. needs parsing
	// TODO
}

//////////////////////////////////////// AJP protocol elements ////////////////////////////////////////