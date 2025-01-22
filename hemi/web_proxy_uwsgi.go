// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// uwsgi reverse proxy (a.k.a. gateway) implementation. See: https://uwsgi-docs.readthedocs.io/en/latest/Protocol.html

// HTTP trailers          : unknown
// Persistent connection  : unknown
// Vague response content : unknown. see: https://uwsgi-docs.readthedocs.io/en/latest/Chunked.html
// Vague request content  : unknown. see: https://uwsgi-docs.readthedocs.io/en/latest/Chunked.html

// uwsgi is now in maintenance mode, see: https://uwsgi-docs.readthedocs.io/en/latest/

package hemi

func init() {
	RegisterHandlet("uwsgiProxy", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(uwsgiProxy)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// uwsgiProxy handlet passes http requests to uWSGI backends and caches responses.
type uwsgiProxy struct {
	// Parent
	Handlet_
	// Assocs
	backend *uwsgiBackend // the backend to pass to
	hcache  Hcache        // the hcache which is used by this proxy
	// States
	WebExchanProxyConfig // embeded
}

func (h *uwsgiProxy) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *uwsgiProxy) OnShutdown() {
	h.webapp.DecSub() // handlet
}

func (h *uwsgiProxy) OnConfigure() {
	// .toBackend
	if v, ok := h.Find("toBackend"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if backend := h.stage.Backend(compName); backend == nil {
				UseExitf("unknown backend: '%s'\n", compName)
			} else if uwsgiBackend, ok := backend.(*uwsgiBackend); ok {
				h.backend = uwsgiBackend
			} else {
				UseExitf("incorrect backend '%s' for uwsgiProxy, must be uwsgiBackend\n", compName)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for uwsgiProxy")
	}

	// .withHcache
	if v, ok := h.Find("withHcache"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if hcache := h.stage.Hcache(compName); hcache == nil {
				UseExitf("unknown hcache: '%s'\n", compName)
			} else {
				h.hcache = hcache
			}
		} else {
			UseExitln("invalid withHcache")
		}
	}

	// .bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.BufferClientContent, true)
	// .bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.BufferServerContent, true)
}
func (h *uwsgiProxy) OnPrepare() {
}

func (h *uwsgiProxy) IsProxy() bool { return true }
func (h *uwsgiProxy) IsCache() bool { return h.hcache != nil }

func (h *uwsgiProxy) Handle(httpReq ServerRequest, httpResp ServerResponse) (handled bool) {
	// TODO: implementation
	httpResp.Send("uwsgi")
	return true
}
