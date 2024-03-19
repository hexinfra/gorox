// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Reverse proxy and related components.

package hemi

import (
	"regexp"
)

// case_ is the parent for *quixCase, *tcpsCase, *udpsCase.
type case_[R Server] struct {
	// Parent
	Component_
	// Assocs
	router R // associated router
	// States
	general  bool   // general match?
	varCode  int16  // the variable code
	varName  string // the variable name
	patterns [][]byte
	regexps  []*regexp.Regexp
}

func (c *case_[R]) onCreate(name string, router R) {
	c.MakeComp(name)
	c.router = router
}
func (c *case_[R]) OnShutdown() {
	c.router.DecSub()
}

func (c *case_[R]) OnConfigure() {
	if c.info == nil {
		c.general = true
		return
	}
	cond := c.info.(caseCond)
	c.varCode = cond.varCode
	c.varName = cond.varName
	isRegexp := cond.compare == "~=" || cond.compare == "!~"
	for _, pattern := range cond.patterns {
		if pattern == "" {
			UseExitln("empty case cond pattern")
		}
		if !isRegexp {
			c.patterns = append(c.patterns, []byte(pattern))
		} else if exp, err := regexp.Compile(pattern); err == nil {
			c.regexps = append(c.regexps, exp)
		} else {
			UseExitln(err.Error())
		}
	}
}
func (c *case_[R]) OnPrepare() {
	// Currently nothing.
}
