// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// CGI handlets starts a CGI program to handle the request and gives a response. See RFC 3875.

package hemi

func init() {
	RegisterHandlet("cgi", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(faviconHandlet)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// cgiHandlet
type cgiHandlet struct {
	// Parent
	Handlet_
	// States
}

func (h *cgiHandlet) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *cgiHandlet) OnShutdown() {
	h.webapp.DecHandlet()
}

func (h *cgiHandlet) OnConfigure() {}
func (h *cgiHandlet) OnPrepare()   {}

func (h *cgiHandlet) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	// TODO
	resp.Send("cgi")
	return true
}
