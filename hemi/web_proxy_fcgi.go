// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// FCGI reverse proxy (a.k.a. gateway) implementation. See: https://fastcgi-archives.github.io/FastCGI_Specification.html

// HTTP trailers          : not supported
// Persistent connection  : supported
// Vague response content : supported through its framing protocol
// Vague request content  : supported, but currently not implemented due to the limitation of CGI/1.1 even though FCGI can do that through its framing protocol

// FCGI is like HTTP/2. To avoid ambiguity, the term "content" in FCGI specification is called "payload" in our implementation.

package hemi

import (
	"errors"
)

func init() {
	RegisterHandlet("fcgiProxy", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(fcgiProxy)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// fcgiProxy handlet passes http requests to FCGI backends and caches responses.
type fcgiProxy struct {
	// Parent
	Handlet_
	// Assocs
	backend *FCGIBackend // the backend to pass to
	hcache  Hcache       // the hcache which is used by this proxy
	// States
	FCGIExchanProxyConfig // embeded
}

func (h *fcgiProxy) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *fcgiProxy) OnShutdown() {
	h.webapp.DecSub() // handlet
}

func (h *fcgiProxy) OnConfigure() {
	// .toBackend
	if v, ok := h.Find("toBackend"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if backend := h.stage.Backend(compName); backend == nil {
				UseExitf("unknown backend: '%s'\n", compName)
			} else if fcgiBackend, ok := backend.(*FCGIBackend); ok {
				h.backend = fcgiBackend
			} else {
				UseExitf("incorrect backend '%s' for fcgiProxy, must be fcgiBackend\n", compName)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for fcgiProxy")
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

	// .scriptFilename
	h.ConfigureBytes("scriptFilename", &h.ScriptFilename, nil, nil)

	// .indexFile
	h.ConfigureBytes("indexFile", &h.IndexFile, func(value []byte) error {
		if len(value) > 0 {
			return nil
		}
		return errors.New(".indexFile has an invalid value")
	}, []byte("index.php"))
}
func (h *fcgiProxy) OnPrepare() {
}

func (h *fcgiProxy) IsProxy() bool { return true }
func (h *fcgiProxy) IsCache() bool { return h.hcache != nil }

func (h *fcgiProxy) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	FCGIExchanReverseProxy(req, resp, h.hcache, h.backend, &h.FCGIExchanProxyConfig)
	return true
}

// FCGIExchanProxyConfig
type FCGIExchanProxyConfig struct {
	WebExchanProxyConfig        // embeded
	ScriptFilename       []byte // for SCRIPT_FILENAME
	IndexFile            []byte // the file that will be used as index
}

// FCGIExchanReverseProxy
func FCGIExchanReverseProxy(httpReq ServerRequest, httpResp ServerResponse, hcache Hcache, backend *FCGIBackend, proxyConfig *FCGIExchanProxyConfig) {
	var httpContent any // nil, []byte, tempFile
	httpHasContent := httpReq.HasContent()
	if httpHasContent && (proxyConfig.BufferClientContent || httpReq.IsVague()) { // including size 0
		httpContent = httpReq.proxyTakeContent()
		if httpContent == nil { // take failed
			// httpStream was marked as broken
			httpResp.SetStatus(StatusBadRequest)
			httpResp.SendBytes(nil)
			return
		}
	}

	fcgiExchan, fcgiErr := backend.fetchExchan(httpReq)
	if fcgiErr != nil {
		httpResp.SendBadGateway(nil)
		return
	}
	defer backend.storeExchan(fcgiExchan)

	fcgiReq := &fcgiExchan.request
	if !fcgiReq.proxyCopyHeaders(httpReq, proxyConfig) {
		fcgiExchan.markBroken()
		httpResp.SendBadGateway(nil)
		return
	}
	if httpHasContent && !proxyConfig.BufferClientContent && !httpReq.IsVague() {
		fcgiErr = fcgiReq.proxyPassMessage(httpReq)
	} else {
		fcgiErr = fcgiReq.proxyPostMessage(httpContent)
	}
	if fcgiErr != nil {
		fcgiExchan.markBroken()
		httpResp.SendBadGateway(nil)
		return
	}

	fcgiResp := &fcgiExchan.response
	for { // until we found a non-1xx status (>= 200)
		fcgiResp.recvHead()
		if fcgiResp.HeadResult() != StatusOK || fcgiResp.status == StatusSwitchingProtocols { // webSocket is not served in handlets.
			fcgiExchan.markBroken()
			httpResp.SendBadGateway(nil)
			return
		}
		if fcgiResp.status >= StatusOK {
			break
		}
		// We got 1xx
		if httpReq.VersionCode() == Version1_0 { // 1xx response is not supported by HTTP/1.0
			fcgiExchan.markBroken()
			httpResp.SendBadGateway(nil)
			return
		}
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !httpResp.proxyPass1xx(fcgiResp) {
			fcgiExchan.markBroken()
			return
		}
		fcgiResp.reuse()
	}

	var fcgiContent any
	fcgiHasContent := false // TODO: if fcgi server includes a content even for HEAD method, what should we do?
	if !httpReq.IsHEAD() {
		fcgiHasContent = fcgiResp.HasContent()
	}
	if fcgiHasContent && proxyConfig.BufferServerContent { // including size 0
		fcgiContent = fcgiResp.proxyTakeContent()
		if fcgiContent == nil { // take failed
			// fcgiExchan was marked as broken
			httpResp.SendBadGateway(nil)
			return
		}
	}

	if !httpResp.proxyCopyHeaders(fcgiResp, &proxyConfig.WebExchanProxyConfig) {
		fcgiExchan.markBroken()
		return
	}
	if fcgiHasContent && !proxyConfig.BufferServerContent {
		if err := httpResp.proxyPassMessage(fcgiResp); err != nil {
			fcgiExchan.markBroken()
			return
		}
	} else if err := httpResp.proxyPostMessage(fcgiContent, false); err != nil { // false means no trailers
		return
	}
}
