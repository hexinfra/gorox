// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UWSGI proxy implementation.

// UWSGI is mainly for Python applications. See: https://uwsgi-docs.readthedocs.io/en/latest/Protocol.html
// UWSGI 1.9.13 seems to have vague content support: https://uwsgi-docs.readthedocs.io/en/latest/Chunked.html

package hemi

import (
	"errors"
	"sync"
	"time"
)

func init() {
	RegisterHandlet("uwsgiProxy", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(uwsgiProxy)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// uwsgiProxy handlet passes web requests to backend uWSGI servers and cache responses.
type uwsgiProxy struct {
	// Mixins
	Handlet_
	contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage       // current stage
	webapp  *Webapp      // the webapp to which the proxy belongs
	backend *TCPSBackend // the backend to pass to
	cacher  Cacher       // the cacher which is used by this proxy
	// States
	bufferClientContent bool          // client content is buffered anyway?
	bufferServerContent bool          // server content is buffered anyway?
	sendTimeout         time.Duration // timeout to send the whole request
	recvTimeout         time.Duration // timeout to recv the whole response content
	maxContentSize      int64         // max response content size allowed
}

func (h *uwsgiProxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *uwsgiProxy) OnShutdown() {
	h.webapp.SubDone()
}

func (h *uwsgiProxy) OnConfigure() {
	h.contentSaver_.onConfigure(h, TmpsDir()+"/web/uwsgi/"+h.name)
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
func (h *uwsgiProxy) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *uwsgiProxy) IsProxy() bool { return true }
func (h *uwsgiProxy) IsCache() bool { return h.cacher != nil }

func (h *uwsgiProxy) Handle(req Request, resp Response) (handled bool) {
	// TODO: implementation
	resp.Send("uwsgi")
	return true
}

// poolUWSGIExchan
var poolUWSGIExchan sync.Pool

func getUWSGIExchan(proxy *uwsgiProxy, conn *TConn) *uwsgiExchan {
	var exchan *uwsgiExchan
	if x := poolUWSGIExchan.Get(); x == nil {
		exchan = new(uwsgiExchan)
		req, resp := &exchan.request, &exchan.response
		req.exchan = exchan
		req.response = resp
		resp.exchan = exchan
	} else {
		exchan = x.(*uwsgiExchan)
	}
	exchan.onUse(proxy, conn)
	return exchan
}
func putUWSGIExchan(exchan *uwsgiExchan) {
	exchan.onEnd()
	poolUWSGIExchan.Put(exchan)
}

// uwsgiExchan
type uwsgiExchan struct {
	// Mixins
	Stream_
	// Assocs
	request  uwsgiRequest  // the uwsgi request
	response uwsgiResponse // the uwsgi response
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	proxy *uwsgiProxy // associated proxy
	conn  *TConn      // associated conn
	// Exchan states (zeros)
}

func (x *uwsgiExchan) onUse(proxy *uwsgiProxy, conn *TConn) {
	x.Stream_.onUse()
	x.proxy = proxy
	x.conn = conn
	x.region.Init()
	x.request.onUse()
	x.response.onUse()
}
func (x *uwsgiExchan) onEnd() {
	x.request.onEnd()
	x.response.onEnd()
	x.conn = nil
	x.proxy = nil
	x.Stream_.onEnd()
}

func (x *uwsgiExchan) buffer256() []byte          { return x.stockBuffer[:] }
func (x *uwsgiExchan) unsafeMake(size int) []byte { return x.region.Make(size) }

// uwsgiRequest
type uwsgiRequest struct { // outgoing. needs building
	// Assocs
	exchan   *uwsgiExchan
	response *uwsgiResponse
}

func (r *uwsgiRequest) onUse() {
	// TODO
}
func (r *uwsgiRequest) onEnd() {
	// TODO
}

// uwsgiResponse
type uwsgiResponse struct { // incoming. needs parsing
	// Assocs
	exchan *uwsgiExchan
}

func (r *uwsgiResponse) onUse() {
	// TODO
}
func (r *uwsgiResponse) onEnd() {
	// TODO
}

//////////////////////////////////////// UWSGI protocol elements ////////////////////////////////////////