// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS network mesher.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// UDPSMesher
type UDPSMesher struct {
	// Mixins
	mesher_[*UDPSMesher, *udpsGate, UDPSFilter, *udpsCase]
}

func (m *UDPSMesher) onCreate(name string, stage *Stage) {
	m.mesher_.onCreate(name, stage, udpsFilterCreators)
}
func (m *UDPSMesher) OnShutdown() {
	// We don't close(m.Shut) here.
	for _, gate := range m.gates {
		gate.shut()
	}
}

func (m *UDPSMesher) OnConfigure() {
	m.mesher_.onConfigure()
	// TODO: configure m
	m.configureSubs()
}
func (m *UDPSMesher) OnPrepare() {
	m.mesher_.onPrepare()
	// TODO: prepare m
	m.prepareSubs()
}

func (m *UDPSMesher) createCase(name string) *udpsCase {
	if m.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(udpsCase)
	kase.onCreate(name, m)
	kase.setShell(kase)
	m.cases = append(m.cases, kase)
	return kase
}

func (m *UDPSMesher) serve() { // goroutine
	for id := int32(0); id < m.numGates; id++ {
		gate := new(udpsGate)
		gate.init(m, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		m.gates = append(m.gates, gate)
		m.IncSub(1)
		if m.tlsMode {
			go gate.serveTLS()
		} else {
			go gate.serveUDP()
		}
	}
	m.WaitSubs() // gates
	m.IncSub(len(m.filters) + len(m.cases))
	m.shutdownSubs()
	m.WaitSubs() // filters, cases

	if m.logger != nil {
		m.logger.Close()
	}
	if Debug() >= 2 {
		Printf("udpsMesher=%s done\n", m.Name())
	}
	m.stage.SubDone()
}

// udpsGate is an opening gate of UDPSMesher.
type udpsGate struct {
	// Mixins
	// Assocs
	stage  *Stage // current stage
	mesher *UDPSMesher
	// States
	id      int32
	address string
	isShut  atomic.Bool
}

func (g *udpsGate) init(mesher *UDPSMesher, id int32) {
	g.stage = mesher.stage
	g.mesher = mesher
	g.id = id
	g.address = mesher.address
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
	g.mesher.SubDone()
}
func (g *udpsGate) serveTLS() { // goroutine
	// TODO
	for !g.isShut.Load() {
		time.Sleep(time.Second)
	}
	g.mesher.SubDone()
}

func (g *udpsGate) execute(link *UDPSLink) { // goroutine
	for _, kase := range g.mesher.cases {
		if !kase.isMatch(link) {
			continue
		}
		if processed := kase.execute(link); processed {
			break
		}
	}
	link.closeConn()
	putUDPSLink(link)
}

func (g *udpsGate) justClose(udpConn *net.UDPConn) {
	udpConn.Close()
}

// UDPSFilter
type UDPSFilter interface {
	// Imports
	Component
	// Methods
	Deal(link *UDPSLink) (next bool)
}

// UDPSFilter_
type UDPSFilter_ struct {
	// Mixins
	Component_
	// States
}

// udpsCase
type udpsCase struct {
	// Mixins
	case_[*UDPSMesher, UDPSFilter]
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
	value := link.unsafeVariable(c.varIndex)
	return c.matcher(c, link, value)
}

var udpsCaseMatchers = map[string]func(kase *udpsCase, link *UDPSLink, value []byte) bool{
	"==": (*udpsCase).equalMatch,
	"^=": (*udpsCase).prefixMatch,
	"$=": (*udpsCase).suffixMatch,
	"*=": (*udpsCase).containMatch,
	"~=": (*udpsCase).regexpMatch,
	"!=": (*udpsCase).notEqualMatch,
	"!^": (*udpsCase).notPrefixMatch,
	"!$": (*udpsCase).notSuffixMatch,
	"!*": (*udpsCase).notContainMatch,
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
func (c *udpsCase) containMatch(link *UDPSLink, value []byte) bool { // value *= patterns
	return c.case_.containMatch(value)
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
func (c *udpsCase) notContainMatch(link *UDPSLink, value []byte) bool { // value !* patterns
	return c.case_.notContainMatch(value)
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

func getUDPSLink(id int64, stage *Stage, mesher *UDPSMesher, gate *udpsGate, udpConn *net.UDPConn, rawConn syscall.RawConn) *UDPSLink {
	var link *UDPSLink
	if x := poolUDPSLink.Get(); x == nil {
		link = new(UDPSLink)
	} else {
		link = x.(*UDPSLink)
	}
	link.onGet(id, stage, mesher, gate, udpConn, rawConn)
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
	mesher  *UDPSMesher
	gate    *udpsGate
	udpConn *net.UDPConn
	rawConn syscall.RawConn
	// Link states (zeros)
}

func (l *UDPSLink) onGet(id int64, stage *Stage, mesher *UDPSMesher, gate *udpsGate, udpConn *net.UDPConn, rawConn syscall.RawConn) {
	l.id = id
	l.stage = stage
	l.mesher = mesher
	l.gate = gate
	l.udpConn = udpConn
	l.rawConn = rawConn
}
func (l *UDPSLink) onPut() {
	l.stage = nil
	l.mesher = nil
	l.gate = nil
	l.udpConn = nil
	l.rawConn = nil
}

func (l *UDPSLink) Close() error {
	udpConn := l.udpConn
	putUDPSLink(l)
	return udpConn.Close()
}

func (l *UDPSLink) closeConn() {
	l.udpConn.Close()
}

func (l *UDPSLink) unsafeVariable(index int16) (value []byte) {
	return udpsLinkVariables[index](l)
}

// udpsLinkVariables
var udpsLinkVariables = [...]func(*UDPSLink) []byte{ // keep sync with varIndexes in config.go
	// TODO
}
