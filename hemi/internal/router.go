// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General router implementation. Router is designed for network proxy, especially service mesh.

package internal

import (
	"bytes"
	"github.com/hexinfra/gorox/hemi/libraries/logger"
	"os"
	"path/filepath"
)

type _gate interface {
	open() error
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

// router_ is the mixin for all routers.
type router_[T Component, G _gate, R _runner, F _filter, C _case] struct {
	// Mixins
	office_
	// Assocs
	gates   []G         // gates opened
	runners compDict[R] // defined runners. indexed by name
	filters compDict[F] // defined filters. indexed by name
	cases   compList[C] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	runnerCreators map[string]func(name string, stage *Stage, router T) R
	filterCreators map[string]func(name string, stage *Stage, router T) F
	filtersByID    [256]F // for fast searching. position 0 is not used
	nFilters       uint8  // used number of filtersByID in this router
	logFile        string
	logger         *logger.Logger
}

func (r *router_[T, G, R, F, C]) init(name string, stage *Stage, runnerCreators map[string]func(string, *Stage, T) R, filterCreators map[string]func(string, *Stage, T) F) {
	r.office_.init(name, stage)
	r.runners = make(compDict[R])
	r.filters = make(compDict[F])
	r.runnerCreators = runnerCreators
	r.filterCreators = filterCreators
	r.nFilters = 1 // position 0 is not used
}

func (r *router_[T, G, R, F, C]) onConfigure() {
	r.office_.onConfigure()
	// logFile
	r.ConfigureString("logFile", &r.logFile, func(value string) bool { return value != "" }, LogsDir()+"/quic_"+r.name+".log")
}
func (r *router_[T, G, R, F, C]) configureSubs() {
	r.runners.walk(R.OnConfigure)
	r.filters.walk(F.OnConfigure)
	r.cases.walk(C.OnConfigure)
}

func (r *router_[T, G, R, F, C]) onPrepare() {
	r.office_.onPrepare()
	// logger
	if err := os.MkdirAll(filepath.Dir(r.logFile), 0755); err != nil {
		EnvExitln(err.Error())
	}
}
func (r *router_[T, G, R, F, C]) prepareSubs() {
	r.runners.walk(R.OnPrepare)
	r.filters.walk(F.OnPrepare)
	r.cases.walk(C.OnPrepare)
}

func (r *router_[T, G, R, F, C]) onShutdown() {
	r.office_.onShutdown()
}
func (r *router_[T, G, R, F, C]) shutdownSubs() {
	r.cases.walk(C.OnShutdown)
	r.filters.walk(F.OnShutdown)
	r.runners.walk(R.OnShutdown)
}

func (r *router_[T, G, R, F, C]) createRunner(sign string, name string) R {
	if _, ok := r.runners[name]; ok {
		UseExitln("conflicting runner with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := r.runnerCreators[sign]
	if !ok {
		UseExitln("unknown runner sign: " + sign)
	}
	runner := create(name, r.stage, r.shell.(T))
	runner.setShell(runner)
	r.runners[name] = runner
	return runner
}
func (r *router_[T, G, R, F, C]) createFilter(sign string, name string) F {
	if r.nFilters == 255 {
		UseExitln("cannot create filter: too many filters in one router")
	}
	if _, ok := r.filters[name]; ok {
		UseExitln("conflicting filter with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := r.filterCreators[sign]
	if !ok {
		UseExitln("unknown filter sign: " + sign)
	}
	filter := create(name, r.stage, r.shell.(T))
	filter.setShell(filter)
	filter.setID(r.nFilters)
	r.filters[name] = filter
	r.filtersByID[r.nFilters] = filter
	r.nFilters++
	return filter
}

func (r *router_[T, G, R, F, C]) filterByID(id uint8) F { // for fast searching
	return r.filtersByID[id]
}

// case_ is a mixin.
type case_[T Component, R _runner, F _filter] struct {
	// Mixins
	Component_
	// Assocs
	router  T   // associated router
	runners []R // runners contained
	filters []F // filters contained
	// States
	general  bool  // general match?
	varCode  int16 // the variable code
	patterns [][]byte
}

func (c *case_[T, R, F]) init(name string, router T) {
	c.SetName(name)
	c.router = router
}

func (c *case_[T, R, F]) OnConfigure() {
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
func (c *case_[T, R, F]) OnPrepare() {
}
func (c *case_[T, R, F]) OnShutdown() {
}

func (c *case_[T, R, F]) addRunner(runner R) {
	c.runners = append(c.runners, runner)
}
func (c *case_[T, R, F]) addFilter(filter F) {
	c.filters = append(c.filters, filter)
}

func (c *case_[T, R, F]) equalMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[T, R, F]) prefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[T, R, F]) suffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[T, R, F]) wildcardMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[T, R, F]) regexpMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[T, R, F]) notEqualMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[T, R, F]) notPrefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[T, R, F]) notSuffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[T, R, F]) notWildcardMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[T, R, F]) notRegexpMatch(value []byte) bool {
	// TODO
	return false
}
