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
type mesher_[M _mesher, G _gate, F _filter, C _case] struct {
	// Mixins
	Server_
	// Assocs
	gates   []G         // a mesher has many gates opened
	filters compDict[F] // defined filters. indexed by name
	cases   compList[C] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	filterCreators map[string]func(name string, stage *Stage, mesher M) F
	accessLog      *logcfg // ...
	logger         *logger // mesher access logger
}

func (m *mesher_[M, G, F, C]) onCreate(name string, stage *Stage, filterCreators map[string]func(string, *Stage, M) F) {
	m.Server_.OnCreate(name, stage)
	m.filters = make(compDict[F])
	m.filterCreators = filterCreators
}

func (m *mesher_[M, G, F, C]) shutdownSubs() { // cases, filters
	m.cases.walk(C.OnShutdown)
	m.filters.walk(F.OnShutdown)
}

func (m *mesher_[M, G, F, C]) onConfigure() {
	m.Server_.OnConfigure()

	// accessLog, TODO
}
func (m *mesher_[M, G, F, C]) configureSubs() { // filters, cases
	m.filters.walk(F.OnConfigure)
	m.cases.walk(C.OnConfigure)
}

func (m *mesher_[M, G, F, C]) onPrepare() {
	m.Server_.OnPrepare()
	if m.accessLog != nil {
		//m.logger = newLogger(m.accessLog.logFile, m.accessLog.rotate)
	}
}
func (m *mesher_[M, G, F, C]) prepareSubs() { // filters, cases
	m.filters.walk(F.OnPrepare)
	m.cases.walk(C.OnPrepare)
}

func (m *mesher_[M, G, F, C]) createFilter(sign string, name string) F {
	if _, ok := m.filters[name]; ok {
		UseExitln("conflicting filter with a same name in mesher")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := m.filterCreators[sign]
	if !ok {
		UseExitln("unknown filter sign: " + sign)
	}
	filter := create(name, m.stage, m.shell.(M))
	filter.setShell(filter)
	m.filters[name] = filter
	return filter
}
func (m *mesher_[M, G, F, C]) hasCase(name string) bool {
	for _, kase := range m.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (m *mesher_[M, G, F, C]) Log(str string) {
	if m.logger != nil {
		m.logger.Log(str)
	}
}
func (m *mesher_[M, G, F, C]) Logln(str string) {
	if m.logger != nil {
		m.logger.Logln(str)
	}
}
func (m *mesher_[M, G, F, C]) Logf(format string, args ...any) {
	if m.logger != nil {
		m.logger.Logf(format, args...)
	}
}

// _gate is the interface for *quicGate, *tcpsGate, *udpsGate.
type _gate interface {
	open() error
	shut() error
}

// _filter is for QUICFilter, TCPSFilter, UDPSFilter.
type _filter interface {
	Component
}

// _case is the interface for *quicCase, *tcpsCase, *udpsCase.
type _case interface {
	Component
}

// case_ is the mixin for *quicCase, *tcpsCase, *udpsCase.
type case_[M _mesher, F _filter] struct {
	// Mixins
	Component_
	// Assocs
	mesher  M   // associated mesher
	filters []F // filters contained
	// States
	general  bool   // general match?
	varCode  int16  // the variable code
	varName  string // the variable name
	patterns [][]byte
	regexps  []*regexp.Regexp
}

func (c *case_[M, F]) onCreate(name string, mesher M) {
	c.MakeComp(name)
	c.mesher = mesher
}
func (c *case_[M, F]) OnShutdown() {
	c.mesher.SubDone()
}

func (c *case_[M, F]) OnConfigure() {
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
func (c *case_[M, F]) OnPrepare() {
}

func (c *case_[M, F]) addFilter(filter F) { c.filters = append(c.filters, filter) }

func (c *case_[M, F]) _equalMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, F]) _prefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, F]) _suffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, F]) _containMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Contains(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, F]) _regexpMatch(value []byte) bool {
	for _, regexp := range c.regexps {
		if regexp.Match(value) {
			return true
		}
	}
	return false
}
func (c *case_[M, F]) _notEqualMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, F]) _notPrefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, F]) _notSuffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, F]) _notContainMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Contains(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, F]) _notRegexpMatch(value []byte) bool {
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
