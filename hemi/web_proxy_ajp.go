// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// AJP proxy implementation.

package hemi

import (
	"errors"
	"sync"
	"time"
)

func init() {
	RegisterHandlet("ajpProxy", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(ajpProxy)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// ajpProxy handlet passes web requests to backend AJP servers and cache responses.
type ajpProxy struct {
	// Parent
	Handlet_
	// Mixins
	_contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage       // current stage
	webapp  *Webapp      // the webapp to which the proxy belongs
	backend *TCPSBackend // the backend to pass to
	cacher  Cacher       // the cacher which is used by this proxy
	// States
	bufferClientContent   bool          // client content is buffered anyway?
	bufferServerContent   bool          // server content is buffered anyway?
	sendTimeout           time.Duration // timeout to send the whole request
	recvTimeout           time.Duration // timeout to recv the whole response content
	maxContentSizeAllowed int64         // max response content size allowed
}

func (h *ajpProxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *ajpProxy) OnShutdown() {
	h.webapp.DecSub()
}

func (h *ajpProxy) OnConfigure() {
	h._contentSaver_.onConfigure(h, TmpsDir()+"/web/ajp/"+h.name)
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

	// maxContentSizeAllowed
	h.ConfigureInt64("maxContentSizeAllowed", &h.maxContentSizeAllowed, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxContentSizeAllowed has an invalid value")
	}, _1T)
}
func (h *ajpProxy) OnPrepare() {
	h._contentSaver_.onPrepare(h, 0755)
}

func (h *ajpProxy) IsProxy() bool { return true }
func (h *ajpProxy) IsCache() bool { return h.cacher != nil }

func (h *ajpProxy) Handle(req Request, resp Response) (handled bool) {
	// TODO: implementation
	resp.Send("ajp")
	return true
}

// poolAJPExchan
var poolAJPExchan sync.Pool

func getAJPExchan(proxy *ajpProxy, conn *TConn) *ajpExchan {
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
	exchan.onUse(proxy, conn)
	return exchan
}
func putAJPExchan(exchan *ajpExchan) {
	exchan.onEnd()
	poolAJPExchan.Put(exchan)
}

// ajpExchan
type ajpExchan struct {
	// Parent
	Stream_
	// Assocs
	request  ajpRequest  // the ajp request
	response ajpResponse // the ajp response
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	proxy *ajpProxy // associated proxy
	conn  *TConn    // associated conn
	// Exchan states (zeros)
}

func (x *ajpExchan) onUse(proxy *ajpProxy, conn *TConn) {
	x.Stream_.onUse()
	x.proxy = proxy
	x.conn = conn
	x.region.Init()
	x.request.onUse()
	x.response.onUse()
}
func (x *ajpExchan) onEnd() {
	x.request.onEnd()
	x.response.onEnd()
	x.conn = nil
	x.proxy = nil
	x.Stream_.onEnd()
}

func (x *ajpExchan) buffer256() []byte          { return x.stockBuffer[:] }
func (x *ajpExchan) unsafeMake(size int) []byte { return x.region.Make(size) }

// ajpRequest
type ajpRequest struct { // outgoing. needs building
	// Assocs
	exchan   *ajpExchan
	response *ajpResponse
}

func (r *ajpRequest) onUse() {
	// TODO
}
func (r *ajpRequest) onEnd() {
	// TODO
}

// ajpResponse must implements the WebBackendResponse interface.
type ajpResponse struct { // incoming. needs parsing
	// Assocs
	exchan *ajpExchan
}

func (r *ajpResponse) onUse() {
	// TODO
}
func (r *ajpResponse) onEnd() {
	// TODO
}

//////////////////////////////////////// AJP protocol elements ////////////////////////////////////////
