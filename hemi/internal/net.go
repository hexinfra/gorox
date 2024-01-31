// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Network mesher and related components.

package internal

import (
	"bytes"
	"regexp"
)

// _mesher is the interface for *QUICMesher, *TCPSMesher, *UDPSMesher.
type _mesher interface {
	Component
	serve() // runner
}

// mesher_ is the mixin for QUICMesher, TCPSMesher, UDPSMesher.
type mesher_[M _mesher, G _gate, D _dealet, C _case] struct {
	// Mixins
	Server_
	// Assocs
	gates   []G         // a mesher has many gates opened
	dealets compDict[D] // defined dealets. indexed by name
	cases   compList[C] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	dealetCreators map[string]func(name string, stage *Stage, mesher M) D
	accessLog      *logcfg // ...
	logger         *logger // mesher access logger
}

func (m *mesher_[M, G, D, C]) onCreate(name string, stage *Stage, dealetCreators map[string]func(string, *Stage, M) D) {
	m.Server_.OnCreate(name, stage)
	m.dealets = make(compDict[D])
	m.dealetCreators = dealetCreators
}

func (m *mesher_[M, G, D, C]) shutdownSubs() { // cases, dealets
	m.cases.walk(C.OnShutdown)
	m.dealets.walk(D.OnShutdown)
}

func (m *mesher_[M, G, D, C]) onConfigure() {
	m.Server_.OnConfigure()

	// accessLog, TODO
}
func (m *mesher_[M, G, D, C]) configureSubs() { // dealets, cases
	m.dealets.walk(D.OnConfigure)
	m.cases.walk(C.OnConfigure)
}

func (m *mesher_[M, G, D, C]) onPrepare() {
	m.Server_.OnPrepare()
	if m.accessLog != nil {
		//m.logger = newLogger(m.accessLog.logFile, m.accessLog.rotate)
	}
}
func (m *mesher_[M, G, D, C]) prepareSubs() { // dealets, cases
	m.dealets.walk(D.OnPrepare)
	m.cases.walk(C.OnPrepare)
}

func (m *mesher_[M, G, D, C]) createDealet(sign string, name string) D {
	if _, ok := m.dealets[name]; ok {
		UseExitln("conflicting dealet with a same name in mesher")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := m.dealetCreators[sign]
	if !ok {
		UseExitln("unknown dealet sign: " + sign)
	}
	dealet := create(name, m.stage, m.shell.(M))
	dealet.setShell(dealet)
	m.dealets[name] = dealet
	return dealet
}
func (m *mesher_[M, G, D, C]) hasCase(name string) bool {
	for _, kase := range m.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (m *mesher_[M, G, D, C]) Log(str string) {
	if m.logger != nil {
		m.logger.Log(str)
	}
}
func (m *mesher_[M, G, D, C]) Logln(str string) {
	if m.logger != nil {
		m.logger.Logln(str)
	}
}
func (m *mesher_[M, G, D, C]) Logf(format string, args ...any) {
	if m.logger != nil {
		m.logger.Logf(format, args...)
	}
}

// _gate is the interface for *quicGate, *tcpsGate, *udpsGate.
type _gate interface {
	open() error
	shut() error
}

// _dealet is for QUICDealet, TCPSDealet, UDPSDealet.
type _dealet interface {
	Component
}

// _case is the interface for *quicCase, *tcpsCase, *udpsCase.
type _case interface {
	Component
}

// case_ is the mixin for *quicCase, *tcpsCase, *udpsCase.
type case_[M _mesher, D _dealet] struct {
	// Mixins
	Component_
	// Assocs
	mesher  M   // associated mesher
	dealets []D // dealets contained
	// States
	general  bool   // general match?
	varCode  int16  // the variable code
	varName  string // the variable name
	patterns [][]byte
	regexps  []*regexp.Regexp
}

func (c *case_[M, D]) onCreate(name string, mesher M) {
	c.MakeComp(name)
	c.mesher = mesher
}
func (c *case_[M, D]) OnShutdown() {
	c.mesher.SubDone()
}

func (c *case_[M, D]) OnConfigure() {
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
func (c *case_[M, D]) OnPrepare() {
}

func (c *case_[M, D]) addDealet(dealet D) { c.dealets = append(c.dealets, dealet) }

func (c *case_[M, D]) _equalMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D]) _prefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D]) _suffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D]) _containMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Contains(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D]) _regexpMatch(value []byte) bool {
	for _, regexp := range c.regexps {
		if regexp.Match(value) {
			return true
		}
	}
	return false
}
func (c *case_[M, D]) _notEqualMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D]) _notPrefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D]) _notSuffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D]) _notContainMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Contains(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D]) _notRegexpMatch(value []byte) bool {
	for _, regexp := range c.regexps {
		if regexp.Match(value) {
			return false
		}
	}
	return true
}

// Buffer
type Buffer struct {
	// TODO
}
