// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
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
	isForward bool // reverse if false
}

func (p *proxy_) onCreate(stage *Stage) {
	p.stage = stage
}

func (p *proxy_) onConfigure(shell Component) {
	// proxyMode
	if v, ok := shell.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok && (mode == "forward" || mode == "reverse") {
			p.isForward = mode == "forward"
		} else {
			UseExitln("invalid proxyMode")
		}
	}
	// toBackend
	if v, ok := shell.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := p.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				p.backend = backend
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else if !p.isForward {
		UseExitln("toBackend is required for reverse proxy")
	}
}
func (p *proxy_) onPrepare(shell Component) {
}
