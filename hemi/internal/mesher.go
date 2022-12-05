// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General mesher implementation. Mesher is designed for network proxy, especially service mesh.

package internal

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
)

type _mesher interface {
	Component
}
type _gate interface {
	open() error
	shutdown() error
}
type _runner interface {
	Component
}
type _filter interface {
	Component
	ider
}
type _case interface {
	Component
}

// mesher_ is the mixin for all meshers.
type mesher_[M _mesher, G _gate, R _runner, F _filter, C _case] struct {
	// Mixins
	Server_
	// Assocs
	gates   []G         // gates opened
	runners compDict[R] // defined runners. indexed by name
	filters compDict[F] // defined filters. indexed by name
	cases   compList[C] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	runnerCreators map[string]func(name string, stage *Stage, mesher M) R
	filterCreators map[string]func(name string, stage *Stage, mesher M) F
	filtersByID    [256]F // for fast searching. position 0 is not used
	nFilters       uint8  // used number of filtersByID in this mesher
	logFile        string // mesher's log file
	logger         *log.Logger
}

func (m *mesher_[M, G, R, F, C]) onCreate(name string, stage *Stage, runnerCreators map[string]func(string, *Stage, M) R, filterCreators map[string]func(string, *Stage, M) F) {
	m.Server_.OnCreate(name, stage)
	m.runners = make(compDict[R])
	m.filters = make(compDict[F])
	m.runnerCreators = runnerCreators
	m.filterCreators = filterCreators
	m.nFilters = 1 // position 0 is not used
}

func (m *mesher_[M, G, R, F, C]) onConfigure() {
	m.Server_.OnConfigure()
	// logFile
	m.ConfigureString("logFile", &m.logFile, func(value string) bool { return value != "" }, LogsDir()+"/mesh-"+m.name+".log")
}
func (m *mesher_[M, G, R, F, C]) configureSubs() {
	m.runners.walk(R.OnConfigure)
	m.filters.walk(F.OnConfigure)
	m.cases.walk(C.OnConfigure)
}

func (m *mesher_[M, G, R, F, C]) onPrepare() {
	m.Server_.OnPrepare()
	// logger
	if err := os.MkdirAll(filepath.Dir(m.logFile), 0755); err != nil {
		EnvExitln(err.Error())
	}
	logFile, err := os.OpenFile(m.logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0700)
	if err != nil {
		EnvExitln(err.Error())
	}
	m.logger = log.New(logFile, "mesher", log.Ldate|log.Ltime)
}
func (m *mesher_[M, G, R, F, C]) prepareSubs() {
	m.runners.walk(R.OnPrepare)
	m.filters.walk(F.OnPrepare)
	m.cases.walk(C.OnPrepare)
}

func (m *mesher_[M, G, R, F, C]) shutdownSubs() {
	m.cases.walk(C.OnShutdown)
	m.filters.walk(F.OnShutdown)
	m.runners.walk(R.OnShutdown)
}

func (m *mesher_[M, G, R, F, C]) createRunner(sign string, name string) R {
	if _, ok := m.runners[name]; ok {
		UseExitln("conflicting runner with a same name in mesher")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := m.runnerCreators[sign]
	if !ok {
		UseExitln("unknown runner sign: " + sign)
	}
	runner := create(name, m.stage, m.shell.(M))
	runner.setShell(runner)
	m.runners[name] = runner
	return runner
}
func (m *mesher_[M, G, R, F, C]) createFilter(sign string, name string) F {
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
func (m *mesher_[M, G, R, F, C]) hasCase(name string) bool {
	for _, kase := range m.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (m *mesher_[M, G, R, F, C]) filterByID(id uint8) F { // for fast searching
	return m.filtersByID[id]
}

// case_ is a mixin.
type case_[M _mesher, R _runner, F _filter] struct {
	// Mixins
	Component_
	// Assocs
	mesher  M   // associated mesher
	runners []R // runners contained
	filters []F // filters contained
	// States
	general  bool  // general match?
	varCode  int16 // the variable code
	patterns [][]byte
}

func (c *case_[M, R, F]) onCreate(name string, mesher M) {
	c.CompInit(name)
	c.mesher = mesher
}

func (c *case_[M, R, F]) OnConfigure() {
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
func (c *case_[M, R, F]) OnPrepare() {
}

func (c *case_[M, R, F]) OnShutdown() {
	c.mesher.SubDone()
}

func (c *case_[M, R, F]) addRunner(runner R) {
	c.runners = append(c.runners, runner)
}
func (c *case_[M, R, F]) addFilter(filter F) {
	c.filters = append(c.filters, filter)
}

func (c *case_[M, R, F]) equalMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, R, F]) prefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, R, F]) suffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, R, F]) wildcardMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[M, R, F]) regexpMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[M, R, F]) notEqualMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, R, F]) notPrefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, R, F]) notSuffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, R, F]) notWildcardMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[M, R, F]) notRegexpMatch(value []byte) bool {
	// TODO
	return false
}
