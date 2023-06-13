// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Network mesher and related components.

package internal

import (
	"bytes"
	"regexp"
)

// _mesher
type _mesher interface { // *QUICMesher, *TCPSMesher, *UDPSMesher
	Component
}

// mesher_ is the mixin for *QUICMesher, *TCPSMesher, *UDPSMesher.
type mesher_[M _mesher, G _gate, D _dealer, C _case] struct {
	// Mixins
	Server_
	// Assocs
	gates   []G         // a mesher has many gates opened
	dealers compDict[D] // defined dealers. indexed by name
	cases   compList[C] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	dealerCreators map[string]func(name string, stage *Stage, mesher M) D
	accessLog      *logcfg // ...
	logger         *logger // mesher access logger
}

func (m *mesher_[M, G, D, C]) onCreate(name string, stage *Stage, dealerCreators map[string]func(string, *Stage, M) D) {
	m.Server_.OnCreate(name, stage)
	m.dealers = make(compDict[D])
	m.dealerCreators = dealerCreators
}

func (m *mesher_[M, G, D, C]) shutdownSubs() { // cases, dealers
	m.cases.walk(C.OnShutdown)
	m.dealers.walk(D.OnShutdown)
}

func (m *mesher_[M, G, D, C]) onConfigure() {
	m.Server_.OnConfigure()

	// accessLog, TODO
}
func (m *mesher_[M, G, D, C]) configureSubs() { // dealers, cases
	m.dealers.walk(D.OnConfigure)
	m.cases.walk(C.OnConfigure)
}

func (m *mesher_[M, G, D, C]) onPrepare() {
	m.Server_.OnPrepare()
	if m.accessLog != nil {
		//m.logger = newLogger(m.accessLog.logFile, m.accessLog.rotate)
	}
}
func (m *mesher_[M, G, D, C]) prepareSubs() { // dealers, cases
	m.dealers.walk(D.OnPrepare)
	m.cases.walk(C.OnPrepare)
}

func (m *mesher_[M, G, D, C]) createDealer(sign string, name string) D {
	if _, ok := m.dealers[name]; ok {
		UseExitln("conflicting dealer with a same name in mesher")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := m.dealerCreators[sign]
	if !ok {
		UseExitln("unknown dealer sign: " + sign)
	}
	dealer := create(name, m.stage, m.shell.(M))
	dealer.setShell(dealer)
	m.dealers[name] = dealer
	return dealer
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

// _gate
type _gate interface { // *quicGate, *tcpsGate, *udpsGate
	open() error
	shut() error
}

// _dealer
type _dealer interface { // QUICDealer, TCPSDealer, UDPSDealer
	Component
}

// _case
type _case interface { // *quicCase, *tcpsCase, *udpsCase
	Component
}

// case_ is a mixin for *quicCase, *tcpsCase, *udpsCase.
type case_[M _mesher, D _dealer] struct {
	// Mixins
	Component_
	// Assocs
	mesher  M   // associated mesher
	dealers []D // dealers contained
	// States
	general  bool  // general match?
	varIndex int16 // the variable index
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
	c.varIndex = cond.varIndex
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

func (c *case_[M, D]) addDealer(dealer D) { c.dealers = append(c.dealers, dealer) }

func (c *case_[M, D]) equalMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D]) prefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D]) suffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D]) containMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Contains(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D]) regexpMatch(value []byte) bool {
	for _, regexp := range c.regexps {
		if regexp.Match(value) {
			return true
		}
	}
	return false
}
func (c *case_[M, D]) notEqualMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D]) notPrefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D]) notSuffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D]) notContainMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Contains(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D]) notRegexpMatch(value []byte) bool {
	for _, regexp := range c.regexps {
		if regexp.Match(value) {
			return false
		}
	}
	return true
}
