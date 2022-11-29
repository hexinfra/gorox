// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General proxy implementation.

package internal

// proxy_ is a mixin for proxies.
type proxy_ struct {
	// Assocs
	stage   *Stage  // current stage
	backend backend // if works as forward proxy, this is nil
	// States
	proxyMode string // forward, reverse
}

func (p *proxy_) onCreate(stage *Stage) {
	p.stage = stage
}

func (p *proxy_) onConfigure(c Component) {
	// proxyMode
	if v, ok := c.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok && (mode == "forward" || mode == "reverse") {
			p.proxyMode = mode
		} else {
			UseExitln("invalid proxyMode")
		}
	} else {
		p.proxyMode = "reverse"
	}
	// toBackend
	if v, ok := c.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := p.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				p.backend = backend
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else if p.proxyMode == "reverse" {
		UseExitln("toBackend is required for reverse proxy")
	}
}
func (p *proxy_) onPrepare() {
}
