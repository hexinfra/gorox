// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// General Network Proxy implementation.

package hemi

import (
	"regexp"
)

// router_ is a parent.
type router_[G Gate] struct { // for QUIXRouter, TCPXRouter, and UDPXRouter
	// Parent
	Server_[G]
	// Mixins
	_accessLogger_
}

func (r *router_[G]) onCreate(compName string, stage *Stage) {
	r.Server_.OnCreate(compName, stage)
}

func (r *router_[G]) onConfigure() {
	r.Server_.OnConfigure()
	r._accessLogger_.onConfigure(r)
}
func (r *router_[G]) onPrepare() {
	r.Server_.OnPrepare()
	r._accessLogger_.onPrepare(r)
}

func (r *router_[G]) DecDealet() { r.subs.Done() }
func (r *router_[G]) DecCase()   { r.subs.Done() }

// case_
type case_ struct { // for quixCase, tcpxCase, and udpxCase
	// Parent
	Component_
	// Assocs
	// States
	general  bool
	varCode  int16
	varName  string
	patterns [][]byte
	regexps  []*regexp.Regexp
}

// dealet_
type dealet_ struct { // for QUIXDealet_, TCPXDealet_, and UDPXDealet_
	// Parent
	Component_
	// Assocs
	stage *Stage
	// States
}

func (d *dealet_) Stage() *Stage { return d.stage }
