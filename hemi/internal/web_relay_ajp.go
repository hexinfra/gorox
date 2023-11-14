// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// AJP relay implementation.

// See: https://tomcat.apache.org/connectors-doc/ajp/ajpv13a.html

// I'm not sure whether AJP supports HTTP unsized content.
// If it doesn't, we have to buffer the request content.

// It seems AJP does support unsized request content, see below:

// Get Body Chunk

// The container asks for more data from the request (If the body was too large to fit in the first packet sent over or when the request is chuncked). The server will send a body packet back with an amount of data which is the minimum of the request_length, the maximum send body size (8186 (8 Kbytes - 6)), and the number of bytes actually left to send from the request body.
// If there is no more data in the body (i.e. the servlet container is trying to read past the end of the body), the server will send back an "empty" packet, which is a body packet with a payload length of 0. (0x12,0x34,0x00,0x00)

package internal

import (
	"errors"
	"sync"
	"time"
)

func init() {
	RegisterHandlet("ajpRelay", func(name string, stage *Stage, app *App) Handlet {
		h := new(ajpRelay)
		h.onCreate(name, stage, app)
		return h
	})
}

// ajpRelay handlet passes web requests to backend AJP servers and cache responses.
type ajpRelay struct {
	// Mixins
	Handlet_
	contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage      // current stage
	app     *App        // the app to which the relay belongs
	backend wireBackend // the *TCPSBackend or *TUDSBackend to pass to
	cacher  Cacher      // the cacher which is used by this relay
	// States
	bufferClientContent bool          // client content is buffered anyway?
	bufferServerContent bool          // server content is buffered anyway?
	sendTimeout         time.Duration // timeout to send the whole request
	recvTimeout         time.Duration // timeout to recv the whole response content
	maxContentSize      int64         // max response content size allowed
}

func (h *ajpRelay) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *ajpRelay) OnShutdown() {
	h.app.SubDone()
}

func (h *ajpRelay) OnConfigure() {
	h.contentSaver_.onConfigure(h, TmpsDir()+"/web/ajp/"+h.name)
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if wireBackend, ok := backend.(wireBackend); ok {
				h.backend = wireBackend
			} else {
				UseExitf("incorrect backend '%s' for ajpRelay, must be TCPSBackend or TUDSBackend\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for ajpRelay")
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
func (h *ajpRelay) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *ajpRelay) IsProxy() bool { return true }
func (h *ajpRelay) IsCache() bool { return h.cacher != nil }

func (h *ajpRelay) Handle(req Request, resp Response) (next bool) { // reverse only
	// TODO: implementation
	resp.Send("ajp")
	return
}

// poolAJPExchan
var poolAJPExchan sync.Pool

func getAJPExchan(relay *ajpRelay, conn wireConn) *ajpExchan {
	var exchan *ajpExchan
	if x := poolAJPExchan.Get(); x == nil {
		exchan = new(ajpExchan)
		req, resp := &exchan.request, &exchan.response
		req.exchan = exchan
		req.response = resp
		resp.exchan = exchan
	} else {
		exchan = x.(*ajpExchan)
	}
	exchan.onUse(relay, conn)
	return exchan
}
func putAJPExchan(exchan *ajpExchan) {
	exchan.onEnd()
	poolAJPExchan.Put(exchan)
}

// ajpExchan
type ajpExchan struct {
	// Assocs
	request  ajpRequest  // the ajp request
	response ajpResponse // the ajp response
}

func (x *ajpExchan) onUse(relay *ajpRelay, conn wireConn) {
}
func (x *ajpExchan) onEnd() {
}

// ajpRequest
type ajpRequest struct { // outgoing. needs building
	// Assocs
	exchan   *ajpExchan
	response *ajpResponse
}

// ajpResponse
type ajpResponse struct { // incoming. needs parsing
	// Assocs
	exchan *ajpExchan
}

//////////////////////////////////////// AJP protocol elements ////////////////////////////////////////
