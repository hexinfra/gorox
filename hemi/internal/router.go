// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General router implementation. Router is designed for network proxy, especially service mesh.

package internal

import (
	"bytes"
)

type _gate interface {
	open() error
	serve()
}
type _runner interface {
	Component
}
type _filter interface {
	Component
	ider
}
type _editor interface {
	Component
	ider
}

// router_ is the mixin for all routers.
type router_[T Component, G _gate, R _runner, F _filter, E _editor] struct {
	// Mixins
	office_
	// Assocs
	gates   []G         // gates opened
	runners compDict[R] // defined runners. indexed by name
	filters compDict[F] // defined filters. indexed by name
	editors compDict[E] // defined editors. indexed by name
	// States
	runnerCreators map[string]func(name string, stage *Stage, router T) R
	filterCreators map[string]func(name string, stage *Stage, router T) F
	editorCreators map[string]func(name string, stage *Stage, router T) E
	filtersByID    [256]F // for fast searching. position 0 is not used
	editorsByID    [256]E // for fast searching. position 0 is not used
	nFilters       uint8  // used number of filtersByID in this router
	nEditors       uint8  // used number of editorsByID in this router
}

func (r *router_[T, G, R, F, E]) init(name string, stage *Stage) {
	r.office_.init(name, stage)
	r.runners = make(compDict[R])
	r.filters = make(compDict[F])
	r.editors = make(compDict[E])
	r.nFilters = 1 // position 0 is not used
	r.nEditors = 1 // position 0 is not used
}
func (r *router_[T, G, R, F, E]) setCreators(runnerCreators map[string]func(string, *Stage, T) R, filterCreators map[string]func(string, *Stage, T) F, editorCreators map[string]func(string, *Stage, T) E) {
	r.runnerCreators = runnerCreators
	r.filterCreators = filterCreators
	r.editorCreators = editorCreators
}

func (r *router_[T, G, R, F, E]) Configure() {
	r.office_.configure()
}
func (r *router_[T, G, R, F, E]) configureSubs() {
	r.runners.walk(R.OnConfigure)
	r.filters.walk(F.OnConfigure)
	r.editors.walk(E.OnConfigure)
}

func (r *router_[T, G, R, F, E]) Prepare() {
	r.office_.prepare()
}
func (r *router_[T, G, R, F, E]) prepareSubs() {
	r.runners.walk(R.OnPrepare)
	r.filters.walk(F.OnPrepare)
	r.editors.walk(E.OnPrepare)
}

func (r *router_[T, G, R, F, E]) Shutdown() {
	r.office_.shutdown()
}
func (r *router_[T, G, R, F, E]) shutdownSubs() {
	r.editors.walk(E.OnShutdown)
	r.filters.walk(F.OnShutdown)
	r.runners.walk(R.OnShutdown)
}

func (r *router_[T, G, R, F, E]) createRunner(sign string, name string) R {
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
func (r *router_[T, G, R, F, E]) createFilter(sign string, name string) F {
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
func (r *router_[T, G, R, F, E]) createEditor(sign string, name string) E {
	if r.nEditors == 255 {
		UseExitln("cannot create editor: too many editors in one router")
	}
	if _, ok := r.editors[name]; ok {
		UseExitln("conflicting editor with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := r.editorCreators[sign]
	if !ok {
		UseExitln("unknown editor sign: " + sign)
	}
	editor := create(name, r.stage, r.shell.(T))
	editor.setShell(editor)
	editor.setID(r.nEditors)
	r.editors[name] = editor
	r.editorsByID[r.nEditors] = editor
	r.nEditors++
	return editor
}

func (r *router_[T, G, R, F, E]) filterByID(id uint8) F { // for fast searching
	return r.filtersByID[id]
}
func (r *router_[T, G, R, F, E]) editorByID(id uint8) E { // for fast searching
	return r.editorsByID[id]
}

// case_ is a mixin.
type case_[T Component, R _runner, F _filter, E _editor] struct {
	// Mixins
	Component_
	// Assocs
	router  T   // belonging router
	runners []R // runners contained
	filters []F // filters contained
	editors []E // editors contained
	// States
	general  bool  // general match?
	varCode  int16 // the variable code
	patterns [][]byte
}

func (c *case_[T, R, F, E]) init(name string, router T) {
	c.SetName(name)
	c.router = router
}

func (c *case_[T, R, F, E]) OnConfigure() {
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
func (c *case_[T, R, F, E]) OnPrepare() {
}
func (c *case_[T, R, F, E]) OnShutdown() {
}

func (c *case_[T, R, F, E]) addRunner(runner R) { c.runners = append(c.runners, runner) }
func (c *case_[T, R, F, E]) addFilter(filter F) { c.filters = append(c.filters, filter) }
func (c *case_[T, R, F, E]) addEditor(editor E) { c.editors = append(c.editors, editor) }

func (c *case_[T, R, F, E]) equalMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[T, R, F, E]) prefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[T, R, F, E]) suffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[T, R, F, E]) wildcardMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[T, R, F, E]) regexpMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[T, R, F, E]) notEqualMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[T, R, F, E]) notPrefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[T, R, F, E]) notSuffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[T, R, F, E]) notWildcardMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[T, R, F, E]) notRegexpMatch(value []byte) bool {
	// TODO
	return false
}
