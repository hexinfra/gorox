// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS router.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// UDPSRouter
type UDPSRouter struct {
	// Mixins
	router_[*UDPSRouter, *udpsGate, UDPSDealer, UDPSEditor, *udpsCase]
}

func (r *UDPSRouter) onCreate(name string, stage *Stage) {
	r.router_.onCreate(name, stage, udpsDealerCreators, udpsEditorCreators)
}
func (r *UDPSRouter) OnShutdown() {
	// We don't close(r.Shut) here.
	for _, gate := range r.gates {
		gate.shut()
	}
}

func (r *UDPSRouter) OnConfigure() {
	r.router_.onConfigure()
	// TODO: configure r
	r.configureSubs()
}
func (r *UDPSRouter) OnPrepare() {
	r.router_.onPrepare()
	// TODO: prepare r
	r.prepareSubs()
}

func (r *UDPSRouter) createCase(name string) *udpsCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(udpsCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}

func (r *UDPSRouter) serve() { // goroutine
	for id := int32(0); id < r.numGates; id++ {
		gate := new(udpsGate)
		gate.init(r, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		r.gates = append(r.gates, gate)
		r.IncSub(1)
		if r.tlsMode {
			go gate.serveTLS()
		} else {
			go gate.serveUDP()
		}
	}
	r.WaitSubs() // gates
	r.IncSub(len(r.dealers) + len(r.editors) + len(r.cases))
	r.shutdownSubs()
	r.WaitSubs() // dealers, editors, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if IsDebug(2) {
		Printf("udpsRouter=%s done\n", r.Name())
	}
	r.stage.SubDone()
}

// udpsGate
type udpsGate struct {
	// Mixins
	// Assocs
	stage  *Stage // current stage
	router *UDPSRouter
	// States
	id      int32
	address string
	isShut  atomic.Bool
}

func (g *udpsGate) init(router *UDPSRouter, id int32) {
	g.stage = router.stage
	g.router = router
	g.id = id
	g.address = router.address
}

func (g *udpsGate) open() error {
	// TODO
	return nil
}
func (g *udpsGate) shut() error {
	g.isShut.Store(true)
	// TODO
	return nil
}

func (g *udpsGate) serveUDP() { // goroutine
	// TODO
	for !g.isShut.Load() {
		time.Sleep(time.Second)
	}
	g.router.SubDone()
}
func (g *udpsGate) serveTLS() { // goroutine
	// TODO
	for !g.isShut.Load() {
		time.Sleep(time.Second)
	}
	g.router.SubDone()
}

func (g *udpsGate) justClose(udpConn *net.UDPConn) {
	udpConn.Close()
}

// UDPSDealer
type UDPSDealer interface {
	Component
	Deal(link *UDPSLink) (next bool)
}

// UDPSDealer_
type UDPSDealer_ struct {
	// Mixins
	Component_
	// States
}

// UDPSEditor
type UDPSEditor interface {
	Component
	identifiable
	OnInput(link *UDPSLink, data []byte) (next bool)
}

// UDPSEditor_
type UDPSEditor_ struct {
	// Mixins
	Component_
	identifiable_
	// States
}

// udpsCase
type udpsCase struct {
	// Mixins
	case_[*UDPSRouter, UDPSDealer, UDPSEditor]
	// States
	matcher func(kase *udpsCase, link *UDPSLink, value []byte) bool
}

func (c *udpsCase) OnConfigure() {
	c.case_.OnConfigure()
	if c.info != nil {
		cond := c.info.(caseCond)
		if matcher, ok := udpsCaseMatchers[cond.compare]; ok {
			c.matcher = matcher
		} else {
			UseExitln("unknown compare in case condition")
		}
	}
}
func (c *udpsCase) OnPrepare() {
	c.case_.OnPrepare()
}

func (c *udpsCase) isMatch(link *UDPSLink) bool {
	if c.general {
		return true
	}
	return c.matcher(c, link, link.unsafeVariable(c.varCode))
}

var udpsCaseMatchers = map[string]func(kase *udpsCase, link *UDPSLink, value []byte) bool{
	"==": (*udpsCase).equalMatch,
	"^=": (*udpsCase).prefixMatch,
	"$=": (*udpsCase).suffixMatch,
	"~=": (*udpsCase).regexpMatch,
	"!=": (*udpsCase).notEqualMatch,
	"!^": (*udpsCase).notPrefixMatch,
	"!$": (*udpsCase).notSuffixMatch,
	"!~": (*udpsCase).notRegexpMatch,
}

func (c *udpsCase) equalMatch(link *UDPSLink, value []byte) bool { // value == patterns
	return c.case_.equalMatch(value)
}
func (c *udpsCase) prefixMatch(link *UDPSLink, value []byte) bool { // value ^= patterns
	return c.case_.prefixMatch(value)
}
func (c *udpsCase) suffixMatch(link *UDPSLink, value []byte) bool { // value $= patterns
	return c.case_.suffixMatch(value)
}
func (c *udpsCase) regexpMatch(link *UDPSLink, value []byte) bool { // value ~= patterns
	return c.case_.regexpMatch(value)
}
func (c *udpsCase) notEqualMatch(link *UDPSLink, value []byte) bool { // value != patterns
	return c.case_.notEqualMatch(value)
}
func (c *udpsCase) notPrefixMatch(link *UDPSLink, value []byte) bool { // value !^ patterns
	return c.case_.notPrefixMatch(value)
}
func (c *udpsCase) notSuffixMatch(link *UDPSLink, value []byte) bool { // value !$ patterns
	return c.case_.notSuffixMatch(value)
}
func (c *udpsCase) notRegexpMatch(link *UDPSLink, value []byte) bool { // value !~ patterns
	return c.case_.notRegexpMatch(value)
}

func (c *udpsCase) execute(link *UDPSLink) (processed bool) {
	// TODO
	return false
}

// poolUDPSLink
var poolUDPSLink sync.Pool

func getUDPSLink(id int64, stage *Stage, router *UDPSRouter, gate *udpsGate, udpConn *net.UDPConn, rawConn syscall.RawConn) *UDPSLink {
	var link *UDPSLink
	if x := poolUDPSLink.Get(); x == nil {
		link = new(UDPSLink)
	} else {
		link = x.(*UDPSLink)
	}
	link.onGet(id, stage, router, gate, udpConn, rawConn)
	return link
}
func putUDPSLink(link *UDPSLink) {
	link.onPut()
	poolUDPSLink.Put(link)
}

// UDPSLink needs redesign, maybe datagram?
type UDPSLink struct {
	// Link states (stocks)
	// Link states (controlled)
	// Link states (non-zeros)
	id      int64
	stage   *Stage // current stage
	router  *UDPSRouter
	gate    *udpsGate
	udpConn *net.UDPConn
	rawConn syscall.RawConn
	// Link states (zeros)
}

func (l *UDPSLink) onGet(id int64, stage *Stage, router *UDPSRouter, gate *udpsGate, udpConn *net.UDPConn, rawConn syscall.RawConn) {
	l.id = id
	l.stage = stage
	l.router = router
	l.gate = gate
	l.udpConn = udpConn
	l.rawConn = rawConn
}
func (l *UDPSLink) onPut() {
	l.stage = nil
	l.router = nil
	l.gate = nil
	l.udpConn = nil
	l.rawConn = nil
}

func (l *UDPSLink) execute() { // goroutine
	for _, kase := range l.router.cases {
		if !kase.isMatch(l) {
			continue
		}
		if processed := kase.execute(l); processed {
			break
		}
	}
	l.closeConn()
	putUDPSLink(l)
}

func (l *UDPSLink) Close() error {
	udpConn := l.udpConn
	putUDPSLink(l)
	return udpConn.Close()
}

func (l *UDPSLink) closeConn() {
	l.udpConn.Close()
}

func (l *UDPSLink) unsafeVariable(index int16) []byte {
	return udpsLinkVariables[index](l)
}

// udpsLinkVariables
var udpsLinkVariables = [...]func(*UDPSLink) []byte{ // keep sync with varCodes in engine.go
	// TODO
}
