// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Reverse proxy and related components.

package hemi

import (
	"bytes"
	"regexp"
)

// router_ is the parent for QUICRouter, TCPSRouter, UDPSRouter.
type router_[R Server, G Gate, D _dealet, C _case] struct {
	// Parent
	Server_[G]
	// Assocs
	dealets compDict[D] // defined dealets. indexed by name
	cases   compList[C] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	dealetCreators map[string]func(name string, stage *Stage, router R) D
	accessLog      *logcfg // ...
	logger         *logger // router access logger
}

func (r *router_[R, G, D, C]) onCreate(name string, stage *Stage, dealetCreators map[string]func(string, *Stage, R) D) {
	r.Server_.OnCreate(name, stage)
	r.dealets = make(compDict[D])
	r.dealetCreators = dealetCreators
}
func (r *router_[R, G, D, C]) onShutdown() {
	r.Server_.OnShutdown()
}

func (r *router_[R, G, D, C]) shutdownSubs() { // cases, dealets
	r.cases.walk(C.OnShutdown)
	r.dealets.walk(D.OnShutdown)
}

func (r *router_[R, G, D, C]) onConfigure() {
	r.Server_.OnConfigure()

	// accessLog, TODO
}
func (r *router_[R, G, D, C]) configureSubs() { // dealets, cases
	r.dealets.walk(D.OnConfigure)
	r.cases.walk(C.OnConfigure)
}

func (r *router_[R, G, D, C]) onPrepare() {
	r.Server_.OnPrepare()

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = newLogger(r.accessLog.logFile, r.accessLog.rotate)
	}
}
func (r *router_[R, G, D, C]) prepareSubs() { // dealets, cases
	r.dealets.walk(D.OnPrepare)
	r.cases.walk(C.OnPrepare)
}

func (r *router_[R, G, D, C]) createDealet(sign string, name string) D {
	if _, ok := r.dealets[name]; ok {
		UseExitln("conflicting dealet with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := r.dealetCreators[sign]
	if !ok {
		UseExitln("unknown dealet sign: " + sign)
	}
	dealet := create(name, r.stage, r.shell.(R))
	dealet.setShell(dealet)
	r.dealets[name] = dealet
	return dealet
}
func (r *router_[R, G, D, C]) hasCase(name string) bool {
	for _, kase := range r.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (r *router_[R, G, D, C]) Log(str string) {
	if r.logger != nil {
		r.logger.Log(str)
	}
}
func (r *router_[R, G, D, C]) Logln(str string) {
	if r.logger != nil {
		r.logger.Logln(str)
	}
}
func (r *router_[R, G, D, C]) Logf(format string, args ...any) {
	if r.logger != nil {
		r.logger.Logf(format, args...)
	}
}

// _dealet is the interface for *QUICDealet, *TCPSDealet, and *UDPSDealet.
type _dealet interface {
	Component
}

// _case is the interface for *quicCase, *tcpsCase, and *udpsCase.
type _case interface {
	Component
}

// case_ is the parent for *quicCase, *tcpsCase, *udpsCase.
type case_[R Server, D _dealet] struct {
	// Parent
	Component_
	// Assocs
	router  R   // associated router
	dealets []D // dealets contained
	// States
	general  bool   // general match?
	varCode  int16  // the variable code
	varName  string // the variable name
	patterns [][]byte
	regexps  []*regexp.Regexp
}

func (c *case_[R, D]) onCreate(name string, router R) {
	c.MakeComp(name)
	c.router = router
}
func (c *case_[R, D]) OnShutdown() {
	c.router.DecSub()
}

func (c *case_[R, D]) OnConfigure() {
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
func (c *case_[R, D]) OnPrepare() {
	// Currently nothing.
}

func (c *case_[R, D]) addDealet(dealet D) { c.dealets = append(c.dealets, dealet) }

func (c *case_[R, D]) _equalMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[R, D]) _prefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[R, D]) _suffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[R, D]) _containMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Contains(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[R, D]) _regexpMatch(value []byte) bool {
	for _, regexp := range c.regexps {
		if regexp.Match(value) {
			return true
		}
	}
	return false
}
func (c *case_[R, D]) _notEqualMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[R, D]) _notPrefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[R, D]) _notSuffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[R, D]) _notContainMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Contains(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[R, D]) _notRegexpMatch(value []byte) bool {
	for _, regexp := range c.regexps {
		if regexp.Match(value) {
			return false
		}
	}
	return true
}
