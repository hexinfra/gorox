// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Pack.

package sitex

import (
	. "github.com/hexinfra/gorox/hemi"
)

// Pack_ is a parent.
type Pack_ struct { // for all packs
	// Assocs
	Site     *Site          // associated site
	Request  ServerRequest  // current request
	Response ServerResponse // current response
	// States
	method    string // GET, POST, HEAD, ...
	action    string // hello, post_new, one_two_three, ...
	forwarded bool
	forwardTo Target
	viewArgs  map[string]value
}

func (p *Pack_) Init(site *Site, req ServerRequest, resp ServerResponse, method string, action string) {
	p.Site = site
	p.Request = req
	p.Response = resp
	p.method = method
	p.action = action
}

func (p *Pack_) ForwardTo(target Target) {
	p.forwarded = true
	p.forwardTo = target
}
func (p *Pack_) Set(k string, v any) {
	if p.viewArgs == nil {
		p.viewArgs = make(map[string]value)
	}
	val := value{}
	p.viewArgs[k] = val
}
func (p *Pack_) Render() error {
	return nil
}
