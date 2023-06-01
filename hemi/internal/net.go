// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Network mesher and related components

package internal

import (
	"bytes"
)

type _mesher interface { // *QUICMesher, *TCPSMesher, *UDPSMesher
	Component
}
type _gate interface { // *quicGate, *tcpsGate, *udpsGate
	open() error
	shut() error
}
type _dealer interface { // QUICDealer, TCPSDealer, UDPSDealer
	Component
}
type _editor interface { // QUICEditor, TCPSEditor, UDPSEditor
	Component
	identifiable
}
type _case interface { // *quicCase, *tcpsCase, *udpsCase
	Component
}

// mesher_ is the mixin for *QUICMesher, *TCPSMesher, *UDPSMesher.
type mesher_[M _mesher, G _gate, D _dealer, E _editor, C _case] struct {
	// Mixins
	Server_
	// Assocs
	gates   []G         // gates opened
	dealers compDict[D] // defined dealers. indexed by name
	editors compDict[E] // defined editors. indexed by name
	cases   compList[C] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	dealerCreators map[string]func(name string, stage *Stage, mesher M) D
	editorCreators map[string]func(name string, stage *Stage, mesher M) E
	accessLog      *logcfg // ...
	logger         *logger // mesher access logger
	editorsByID    [256]E  // for fast searching. position 0 is not used
	nEditors       uint8   // used number of editorsByID in this mesher
}

func (r *mesher_[M, G, D, E, C]) onCreate(name string, stage *Stage, dealerCreators map[string]func(string, *Stage, M) D, editorCreators map[string]func(string, *Stage, M) E) {
	r.Server_.OnCreate(name, stage)
	r.dealers = make(compDict[D])
	r.editors = make(compDict[E])
	r.dealerCreators = dealerCreators
	r.editorCreators = editorCreators
	r.nEditors = 1 // position 0 is not used
}

func (r *mesher_[M, G, D, E, C]) shutdownSubs() { // cases, editors, dealers
	r.cases.walk(C.OnShutdown)
	r.editors.walk(E.OnShutdown)
	r.dealers.walk(D.OnShutdown)
}

func (r *mesher_[M, G, D, E, C]) onConfigure() {
	r.Server_.OnConfigure()

	// accessLog, TODO
}
func (r *mesher_[M, G, D, E, C]) configureSubs() { // dealers, editors, cases
	r.dealers.walk(D.OnConfigure)
	r.editors.walk(E.OnConfigure)
	r.cases.walk(C.OnConfigure)
}

func (r *mesher_[M, G, D, E, C]) onPrepare() {
	r.Server_.OnPrepare()
	if r.accessLog != nil {
		//r.logger = newLogger(r.accessLog.logFile, r.accessLog.rotate)
	}
}
func (r *mesher_[M, G, D, E, C]) prepareSubs() { // dealers, editors, cases
	r.dealers.walk(D.OnPrepare)
	r.editors.walk(E.OnPrepare)
	r.cases.walk(C.OnPrepare)
}

func (r *mesher_[M, G, D, E, C]) createDealer(sign string, name string) D {
	if _, ok := r.dealers[name]; ok {
		UseExitln("conflicting dealer with a same name in mesher")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := r.dealerCreators[sign]
	if !ok {
		UseExitln("unknown dealer sign: " + sign)
	}
	dealer := create(name, r.stage, r.shell.(M))
	dealer.setShell(dealer)
	r.dealers[name] = dealer
	return dealer
}
func (r *mesher_[M, G, D, E, C]) createEditor(sign string, name string) E {
	if r.nEditors == 255 {
		UseExitln("cannot create editor: too many editors in one mesher")
	}
	if _, ok := r.editors[name]; ok {
		UseExitln("conflicting editor with a same name in mesher")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := r.editorCreators[sign]
	if !ok {
		UseExitln("unknown editor sign: " + sign)
	}
	editor := create(name, r.stage, r.shell.(M))
	editor.setShell(editor)
	editor.setID(r.nEditors)
	r.editors[name] = editor
	r.editorsByID[r.nEditors] = editor
	r.nEditors++
	return editor
}
func (r *mesher_[M, G, D, E, C]) hasCase(name string) bool {
	for _, kase := range r.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (r *mesher_[M, G, D, E, C]) editorByID(id uint8) E { return r.editorsByID[id] }

func (r *mesher_[M, G, D, E, C]) Log(s string) {
	if r.logger != nil {
		r.logger.Log(s)
	}
}
func (r *mesher_[M, G, D, E, C]) Logln(s string) {
	if r.logger != nil {
		r.logger.Logln(s)
	}
}
func (r *mesher_[M, G, D, E, C]) Logf(format string, args ...any) {
	if r.logger != nil {
		r.logger.Logf(format, args...)
	}
}

// case_ is a mixin for *quicCase, *tcpsCase, *udpsCase.
type case_[M _mesher, D _dealer, E _editor] struct {
	// Mixins
	Component_
	// Assocs
	mesher  M   // associated mesher
	dealers []D // dealers contained
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

func (c *case_[M, D, E]) addDealer(dealer D) { c.dealers = append(c.dealers, dealer) }
func (c *case_[M, D, E]) addEditor(editor E) { c.editors = append(c.editors, editor) }

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
