// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS service router.

package internal

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/logger"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// TCPSRouter
type TCPSRouter struct {
	// Mixins
	router_[*TCPSRouter, *tcpsGate, TCPSRunner, TCPSFilter, TCPSEditor]
	// Assocs
	cases compList[*tcpsCase] // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	logFile string
	logger  *logger.Logger
}

func (r *TCPSRouter) init(name string, stage *Stage) {
	r.router_.init(name, stage)
	r.router_.setCreators(tcpsRunnerCreators, tcpsFilterCreators, tcpsEditorCreators)
}

func (r *TCPSRouter) OnConfigure() {
	r.router_.onConfigure()
	// logFile
	r.ConfigureString("logFile", &r.logFile, func(value string) bool { return value != "" }, LogsDir()+"/tcps_"+r.name+".log")

	// sub components
	r.configureSubs()
	r.cases.walk((*tcpsCase).OnConfigure)
}
func (r *TCPSRouter) OnPrepare() {
	r.router_.onPrepare()
	// logger
	if err := os.MkdirAll(filepath.Dir(r.logFile), 0755); err != nil {
		EnvExitln(err.Error())
	}

	// sub components
	r.prepareSubs()
	r.cases.walk((*tcpsCase).OnPrepare)
}
func (r *TCPSRouter) OnShutdown() {
	r.cases.walk((*tcpsCase).OnShutdown)
	// sub components
	r.shutdownSubs()

	r.router_.onShutdown()
}

func (r *TCPSRouter) createCase(name string) *tcpsCase {
	/*
		if r.Case(name) != nil {
			UseExitln("conflicting case with a same name")
		}
	*/
	kase := new(tcpsCase)
	kase.init(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}

func (r *TCPSRouter) serve() {
	for id := int32(0); id < r.numGates; id++ {
		gate := new(tcpsGate)
		gate.init(r, id)
		if err := gate.open(); err != nil {
			EnvExitln(err.Error())
		}
		r.gates = append(r.gates, gate)
		go gate.serve()
	}
	select {}
}

func (r *TCPSRouter) dispatch(conn *TCPSConn) {
	/*
		for _, kase := range r.cases {
			if !kase.isMatch(conn) {
				continue
			}
			if processed := executeCase[*TCPSConn, tcpsFilter](kase, conn); processed {
				return
			}
		}
	*/
	conn.closeConn()
	putTCPSConn(conn)
}

// tcpsGate
type tcpsGate struct {
	// Mixins
	Gate_
	// Assocs
	router *TCPSRouter
	// States
	listener *net.TCPListener
}

func (g *tcpsGate) init(router *TCPSRouter, id int32) {
	g.Init(router.stage, id, router.address, router.maxConnsPerGate)
	g.router = router
}

func (g *tcpsGate) open() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		return system.SetReusePort(rawConn)
	}
	listener, err := listenConfig.Listen(context.Background(), "tcp", g.address)
	if err == nil {
		g.listener = listener.(*net.TCPListener)
	}
	return err
}

func (g *tcpsGate) serve() {
	if g.router.tlsMode {
		g.serveTLS()
	} else {
		g.serveTCP()
	}
}
func (g *tcpsGate) serveTCP() {
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
		if err != nil {
			continue
		}
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				tcpConn.Close()
				continue
			}
			tcpsConn := getTCPSConn(connID, g.stage, g.router, g, tcpConn, rawConn)
			if IsDebug() {
				fmt.Printf("%+v\n", tcpsConn)
			}
			go g.router.dispatch(tcpsConn) // tcpsConn is put to pool in dispatch()
			connID++
		}
	}
}
func (g *tcpsGate) serveTLS() {
	connID := int64(0)
	for {
		tcpConn, err := g.listener.AcceptTCP()
		if err != nil {
			continue
		}
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.router.tlsConfig)
			// TODO: set deadline
			if err := tlsConn.Handshake(); err != nil {
				tlsConn.Close()
				continue
			}
			tcpsConn := getTCPSConn(connID, g.stage, g.router, g, tlsConn, nil)
			go g.router.dispatch(tcpsConn) // tcpsConn is put to pool in dispatch()
			connID++
		}
	}
}

func (g *tcpsGate) onConnectionClosed() {
	g.DecConns()
}
func (g *tcpsGate) justClose(tcpConn *net.TCPConn) {
	tcpConn.Close()
	g.onConnectionClosed()
}

// TCPSRunner
type TCPSRunner interface {
	Component
	Process(conn *TCPSConn) (next bool)
}

// TCPSRunner_
type TCPSRunner_ struct {
	Component_
}

// TCPSFilter
type TCPSFilter interface {
	Component
	ider
	OnInput(conn *TCPSConn, data []byte) (next bool)
}

// TCPSFilter_
type TCPSFilter_ struct {
	Component_
	ider_
}

// TCPSEditor
type TCPSEditor interface {
	Component
	ider
	OnOutput(conn *TCPSConn, data []byte)
}

// TCPSEditor_
type TCPSEditor_ struct {
	Component_
	ider_
}

// tcpsCase
type tcpsCase struct {
	// Mixins
	case_[*TCPSRouter, TCPSRunner, TCPSFilter, TCPSEditor]
	// States
	matcher func(kase *tcpsCase, conn *TCPSConn, value []byte) bool
}

func (c *tcpsCase) OnConfigure() {
	c.case_.OnConfigure()
	if c.info != nil {
		cond := c.info.(caseCond)
		if matcher, ok := tcpsCaseMatchers[cond.compare]; ok {
			c.matcher = matcher
		} else {
			UseExitln("unknown compare in case condition")
		}
	}
}

func (c *tcpsCase) isMatch(conn *TCPSConn) bool {
	if c.general {
		return true
	}
	return c.matcher(c, conn, conn.unsafeVariable(c.varCode))
}

var tcpsCaseMatchers = map[string]func(kase *tcpsCase, conn *TCPSConn, value []byte) bool{
	"==": (*tcpsCase).equalMatch,
	"^=": (*tcpsCase).prefixMatch,
	"$=": (*tcpsCase).suffixMatch,
	"*=": (*tcpsCase).wildcardMatch,
	"~=": (*tcpsCase).regexpMatch,
	"!=": (*tcpsCase).notEqualMatch,
	"!^": (*tcpsCase).notPrefixMatch,
	"!$": (*tcpsCase).notSuffixMatch,
	"!*": (*tcpsCase).notWildcardMatch,
	"!~": (*tcpsCase).notRegexpMatch,
}

func (c *tcpsCase) equalMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.equalMatch(value)
}
func (c *tcpsCase) prefixMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.prefixMatch(value)
}
func (c *tcpsCase) suffixMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.suffixMatch(value)
}
func (c *tcpsCase) wildcardMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.wildcardMatch(value)
}
func (c *tcpsCase) regexpMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.regexpMatch(value)
}
func (c *tcpsCase) notEqualMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.notEqualMatch(value)
}
func (c *tcpsCase) notPrefixMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.notPrefixMatch(value)
}
func (c *tcpsCase) notSuffixMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.notSuffixMatch(value)
}
func (c *tcpsCase) notWildcardMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.notWildcardMatch(value)
}
func (c *tcpsCase) notRegexpMatch(conn *TCPSConn, value []byte) bool {
	return c.case_.notRegexpMatch(value)
}

// poolTCPSConn
var poolTCPSConn sync.Pool

func getTCPSConn(id int64, stage *Stage, router *TCPSRouter, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) *TCPSConn {
	var conn *TCPSConn
	if x := poolTCPSConn.Get(); x == nil {
		conn = new(TCPSConn)
	} else {
		conn = x.(*TCPSConn)
	}
	conn.onGet(id, stage, router, gate, netConn, rawConn)
	return conn
}
func putTCPSConn(conn *TCPSConn) {
	conn.onPut()
	poolTCPSConn.Put(conn)
}

// TCPSConn
type TCPSConn struct {
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64
	stage   *Stage // current stage
	router  *TCPSRouter
	gate    *tcpsGate
	netConn net.Conn
	rawConn syscall.RawConn
	arena   region
	// Conn states (zeros)
}

func (c *TCPSConn) onGet(id int64, stage *Stage, router *TCPSRouter, gate *tcpsGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.stage = stage
	c.router = router
	c.gate = gate
	c.netConn = netConn
	c.rawConn = rawConn
	c.arena.init()
}
func (c *TCPSConn) onPut() {
	c.stage = nil
	c.router = nil
	c.gate = nil
	c.netConn = nil
	c.rawConn = nil
	c.arena.free()
}

func (c *TCPSConn) Read(p []byte) (n int, err error)  { return c.netConn.Read(p) }
func (c *TCPSConn) Write(p []byte) (n int, err error) { return c.netConn.Write(p) }

func (c *TCPSConn) Close() error {
	netConn := c.netConn
	putTCPSConn(c)
	return netConn.Close()
}

func (c *TCPSConn) closeConn() {
	c.netConn.Close()
	c.gate.onConnectionClosed()
}

func (c *TCPSConn) unsafeVariable(index int16) []byte {
	return tcpsConnVariables[index](c)
}

// tcpsConnVariables
var tcpsConnVariables = [...]func(*TCPSConn) []byte{ // keep sync with varCodes in config.go
	// TODO
}
