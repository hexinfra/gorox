// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// SCGI reverse proxy (a.k.a. gateway) implementation. See: https://python.ca/scgi/protocol.txt

// HTTP trailer section   : not supported
// Persistent connection  : not supported, each request/response exchan occupies a connection
// Vague response content : supported, as scgi servers close the connection after the response has been sent
// Vague request content  : not supported, proxies MUST buffer the entire content and send sized requests to scgi servers

// SCGI protocol doesn't define the format of its response, I don't know why. Seems it follows the format of CGI response.

package hemi

func init() {
	RegisterHandlet("scgiProxy", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(scgiProxy)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// scgiProxy handlet passes http requests to SCGI backends and caches responses.
type scgiProxy struct {
	// Parent
	Handlet_
	// Assocs
	backend *SCGIBackend // the backend to pass to
	hcache  Hcache       // the hcache which is used by this proxy
	// States
	SCGIProxyConfig // embeded
}

func (h *scgiProxy) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *scgiProxy) OnShutdown() { h.webapp.DecHandlet() }

func (h *scgiProxy) OnConfigure() {
	// .toBackend
	if v, ok := h.Find("toBackend"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if backend := h.stage.Backend(compName); backend == nil {
				UseExitf("unknown backend: '%s'\n", compName)
			} else if scgiBackend, ok := backend.(*SCGIBackend); ok {
				h.backend = scgiBackend
			} else {
				UseExitf("incorrect backend '%s' for scgiProxy, must be scgiBackend\n", compName)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for scgiProxy")
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
func (h *scgiProxy) OnPrepare() {
}

func (h *scgiProxy) IsProxy() bool { return true }            // works as a reverse proxy
func (h *scgiProxy) IsCache() bool { return h.hcache != nil } // works as a proxy cache?

func (h *scgiProxy) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	SCGIReverseProxy(req, resp, h.hcache, h.backend, &h.SCGIProxyConfig)
	return true
}

// SCGIProxyConfig
type SCGIProxyConfig struct {
	HTTPProxyConfig // embeded
}

// SCGIReverseProxy
func SCGIReverseProxy(httpReq ServerRequest, httpResp ServerResponse, hcache Hcache, backend *SCGIBackend, proxyConfig *SCGIProxyConfig) {
	// TODO
	httpResp.Send("SCGI")
}
