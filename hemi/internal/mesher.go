// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General mesher implementation. Mesher is designed for network proxy, especially service mesh.

package internal

import (
	"bytes"
)

type _mesher interface {
	Component
}
type _gate interface {
	open() error
	shutdown() error
}
type _dealer interface {
	Component
}
type _filter interface {
	Component
	identifiable
}
type _case interface {
	Component
}

// mesher_ is the mixin for all meshers.
type mesher_[M _mesher, G _gate, D _dealer, F _filter, C _case] struct {
	// Mixins
	Server_
	// Assocs
	gates   []G         // gates opened
	dealers compDict[D] // defined dealers. indexed by name
	filters compDict[F] // defined filters. indexed by name
	cases   compList[C] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	dealerCreators map[string]func(name string, stage *Stage, mesher M) D
	filterCreators map[string]func(name string, stage *Stage, mesher M) F
	accessLog      []string // (file, rotate)
	booker         *booker  // mesher access booker
	filtersByID    [256]F   // for fast searching. position 0 is not used
	nFilters       uint8    // used number of filtersByID in this mesher
}

func (m *mesher_[M, G, D, F, C]) onCreate(name string, stage *Stage, dealerCreators map[string]func(string, *Stage, M) D, filterCreators map[string]func(string, *Stage, M) F) {
	m.Server_.OnCreate(name, stage)
	m.dealers = make(compDict[D])
	m.filters = make(compDict[F])
	m.dealerCreators = dealerCreators
	m.filterCreators = filterCreators
	m.nFilters = 1 // position 0 is not used
}

func (m *mesher_[M, G, D, F, C]) shutdownSubs() { // cases, filters, dealers
	m.cases.walk(C.OnShutdown)
	m.filters.walk(F.OnShutdown)
	m.dealers.walk(D.OnShutdown)
}

func (m *mesher_[M, G, D, F, C]) onConfigure() {
	m.Server_.OnConfigure()
	// accessLog
	if v, ok := m.Find("accessLog"); ok {
		if log, ok := v.StringListN(2); ok {
			m.accessLog = log
		} else {
			UseExitln("invalid accessLog")
		}
	} else {
		m.accessLog = nil
	}
}
func (m *mesher_[M, G, D, F, C]) configureSubs() { // dealers, filters, cases
	m.dealers.walk(D.OnConfigure)
	m.filters.walk(F.OnConfigure)
	m.cases.walk(C.OnConfigure)
}

func (m *mesher_[M, G, D, F, C]) onPrepare() {
	m.Server_.OnPrepare()
	if m.accessLog != nil {
		//m.booker = newBooker(m.accessLog[0], m.accessLog[1])
	}
}
func (m *mesher_[M, G, D, F, C]) prepareSubs() { // dealers, filters, cases
	m.dealers.walk(D.OnPrepare)
	m.filters.walk(F.OnPrepare)
	m.cases.walk(C.OnPrepare)
}

func (m *mesher_[M, G, D, F, C]) createDealer(sign string, name string) D {
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
func (m *mesher_[M, G, D, F, C]) createFilter(sign string, name string) F {
	if m.nFilters == 255 {
		UseExitln("cannot create filter: too many filters in one mesher")
	}
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
	filter.setID(m.nFilters)
	m.filters[name] = filter
	m.filtersByID[m.nFilters] = filter
	m.nFilters++
	return filter
}
func (m *mesher_[M, G, D, F, C]) hasCase(name string) bool {
	for _, kase := range m.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (m *mesher_[M, G, D, F, C]) filterByID(id uint8) F { // for fast searching
	return m.filtersByID[id]
}

// case_ is a mixin.
type case_[M _mesher, D _dealer, F _filter] struct {
	// Mixins
	Component_
	// Assocs
	mesher  M   // associated mesher
	dealers []D // dealers contained
	filters []F // filters contained
	// States
	general  bool  // general match?
	varCode  int16 // the variable code
	patterns [][]byte
}

func (c *case_[M, D, F]) onCreate(name string, mesher M) {
	c.MakeComp(name)
	c.mesher = mesher
}
func (c *case_[M, D, F]) OnShutdown() {
	c.mesher.SubDone()
}

func (c *case_[M, D, F]) OnConfigure() {
	if c.info == nil {
		c.general = true
		return
	}
	cond := c.info.(caseCond)
	c.varCode = cond.varCode
	for _, pattern := range cond.patterns {
		if pattern == "" {
			UseExitln("empty case cond pattern")
		}
		c.patterns = append(c.patterns, []byte(pattern))
	}
}
func (c *case_[M, D, F]) OnPrepare() {
}

func (c *case_[M, D, F]) addDealer(dealer D) {
	c.dealers = append(c.dealers, dealer)
}
func (c *case_[M, D, F]) addFilter(filter F) {
	c.filters = append(c.filters, filter)
}

func (c *case_[M, D, F]) equalMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D, F]) prefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D, F]) suffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D, F]) regexpMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[M, D, F]) notEqualMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D, F]) notPrefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D, F]) notSuffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D, F]) notRegexpMatch(value []byte) bool {
	// TODO
	return false
}
