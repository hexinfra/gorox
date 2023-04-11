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

// mesher_ is the mixin for all meshers.
type mesher_[M _mesher, G _gate, D _dealet, E _editor, C _case] struct {
	// Mixins
	Server_
	// Assocs
	gates   []G         // gates opened
	dealets compDict[D] // defined dealets. indexed by name
	editors compDict[E] // defined editors. indexed by name
	cases   compList[C] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	dealetCreators map[string]func(name string, stage *Stage, mesher M) D
	editorCreators map[string]func(name string, stage *Stage, mesher M) E
	accessLog      []string // (file, rotate)
	booker         *booker  // mesher access booker
	editorsByID    [256]E   // for fast searching. position 0 is not used
	nEditors       uint8    // used number of editorsByID in this mesher
}

func (m *mesher_[M, G, D, E, C]) onCreate(name string, stage *Stage, dealetCreators map[string]func(string, *Stage, M) D, editorCreators map[string]func(string, *Stage, M) E) {
	m.Server_.OnCreate(name, stage)
	m.dealets = make(compDict[D])
	m.editors = make(compDict[E])
	m.dealetCreators = dealetCreators
	m.editorCreators = editorCreators
	m.nEditors = 1 // position 0 is not used
}

func (m *mesher_[M, G, D, E, C]) shutdownSubs() {
	m.cases.walk(C.OnShutdown)
	m.editors.walk(E.OnShutdown)
	m.dealets.walk(D.OnShutdown)
}

func (m *mesher_[M, G, D, E, C]) onConfigure() {
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
func (m *mesher_[M, G, D, E, C]) configureSubs() {
	m.dealets.walk(D.OnConfigure)
	m.editors.walk(E.OnConfigure)
	m.cases.walk(C.OnConfigure)
}

func (m *mesher_[M, G, D, E, C]) onPrepare() {
	m.Server_.OnPrepare()
	if m.accessLog != nil {
		//m.booker = newBooker(m.accessLog[0], m.accessLog[1])
	}
}
func (m *mesher_[M, G, D, E, C]) prepareSubs() {
	m.dealets.walk(D.OnPrepare)
	m.editors.walk(E.OnPrepare)
	m.cases.walk(C.OnPrepare)
}

func (m *mesher_[M, G, D, E, C]) createDealet(sign string, name string) D {
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
func (m *mesher_[M, G, D, E, C]) createEditor(sign string, name string) E {
	if m.nEditors == 255 {
		UseExitln("cannot create editor: too many editors in one mesher")
	}
	if _, ok := m.editors[name]; ok {
		UseExitln("conflicting editor with a same name in mesher")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := m.editorCreators[sign]
	if !ok {
		UseExitln("unknown editor sign: " + sign)
	}
	editor := create(name, m.stage, m.shell.(M))
	editor.setShell(editor)
	editor.setID(m.nEditors)
	m.editors[name] = editor
	m.editorsByID[m.nEditors] = editor
	m.nEditors++
	return editor
}
func (m *mesher_[M, G, D, E, C]) hasCase(name string) bool {
	for _, kase := range m.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (m *mesher_[M, G, D, E, C]) editorByID(id uint8) E { // for fast searching
	return m.editorsByID[id]
}

// case_ is a mixin.
type case_[M _mesher, D _dealet, E _editor] struct {
	// Mixins
	Component_
	// Assocs
	mesher  M   // associated mesher
	dealets []D // dealets contained
	editors []E // editors contained
	// States
	general  bool  // general match?
	varCode  int16 // the variable code
	patterns [][]byte
}

func (c *case_[M, D, E]) onCreate(name string, mesher M) {
	c.MakeComp(name)
	c.mesher = mesher
}
func (c *case_[M, D, E]) OnShutdown() {
	c.mesher.SubDone()
}

func (c *case_[M, D, E]) OnConfigure() {
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
func (c *case_[M, D, E]) OnPrepare() {
}

func (c *case_[M, D, E]) addDealet(dealet D) {
	c.dealets = append(c.dealets, dealet)
}
func (c *case_[M, D, E]) addEditor(editor E) {
	c.editors = append(c.editors, editor)
}

func (c *case_[M, D, E]) equalMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D, E]) prefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D, E]) suffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func (c *case_[M, D, E]) regexpMatch(value []byte) bool {
	// TODO
	return false
}
func (c *case_[M, D, E]) notEqualMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D, E]) notPrefixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D, E]) notSuffixMatch(value []byte) bool {
	for _, pattern := range c.patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func (c *case_[M, D, E]) notRegexpMatch(value []byte) bool {
	// TODO
	return false
}

type _dealet interface {
	Component
}
type _editor interface {
	Component
	identifiable
}
type _case interface {
	Component
}

type _gate interface {
	open() error
	shutdown() error
}
