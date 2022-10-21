// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Controller.

package sitex

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

// Controller_
type Controller_ struct {
	// Assocs
	Site     *Site    // belonging site
	Request  Request  // current request
	Response Response // current response
	// States
	method    string // GET, POST, HEAD, ...
	action    string // hello, post_new, one_two_three, ...
	forwarded bool
	forwardTo Target
	viewArgs  map[string]value
}

func (c *Controller_) Init(site *Site, req Request, resp Response, method string, action string) {
	c.Site = site
	c.Request = req
	c.Response = resp
	c.method = method
	c.action = action
}

func (c *Controller_) ForwardTo(target Target) {
	c.forwarded = true
	c.forwardTo = target
}
func (c *Controller_) Set(k string, v any) {
	if c.viewArgs == nil {
		c.viewArgs = make(map[string]value)
	}
	val := value{}
	c.viewArgs[k] = val
}
func (c *Controller_) Render() error {
	return nil
}
