// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPX (TCP/TLS/UDS) router and reverse proxy.

package hemi

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hexinfra/gorox/hemi/library/system"
)

func init() {
	RegisterTCPXDealet("tcpxProxy", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(tcpxProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// TCPXRouter
type TCPXRouter struct {
	// Parent
	Server_[*tcpxGate]
	// Assocs
	dealets compDict[TCPXDealet] // defined dealets. indexed by name
	cases   compList[*tcpxCase]  // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	accessLog *logcfg // ...
	logger    *logger // router access logger
}

func (r *TCPXRouter) onCreate(name string, stage *Stage) {
	r.Server_.OnCreate(name, stage)
	r.dealets = make(compDict[TCPXDealet])
}
func (r *TCPXRouter) OnShutdown() {
	r.Server_.OnShutdown()
}

func (r *TCPXRouter) OnConfigure() {
	r.Server_.OnConfigure()

	// accessLog, TODO

	// sub components
	r.dealets.walk(TCPXDealet.OnConfigure)
	r.cases.walk((*tcpxCase).OnConfigure)
}
func (r *TCPXRouter) OnPrepare() {
	r.Server_.OnPrepare()

	// accessLog, TODO
	if r.accessLog != nil {
		//r.logger = newLogger(r.accessLog.logFile, r.accessLog.rotate)
	}

	// sub components
	r.dealets.walk(TCPXDealet.OnPrepare)
	r.cases.walk((*tcpxCase).OnPrepare)
}

func (r *TCPXRouter) createDealet(sign string, name string) TCPXDealet {
	if _, ok := r.dealets[name]; ok {
		UseExitln("conflicting dealet with a same name in router")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := tcpxDealetCreators[sign]
	if !ok {
		UseExitln("unknown dealet sign: " + sign)
	}
	dealet := create(name, r.stage, r)
	dealet.setShell(dealet)
	r.dealets[name] = dealet
	return dealet
}
func (r *TCPXRouter) createCase(name string) *tcpxCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(tcpxCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}
func (r *TCPXRouter) hasCase(name string) bool {
	for _, kase := range r.cases {
		if kase.Name() == name {
			return true
		}
	}
	return false
}

func (r *TCPXRouter) Log(str string) {
}
func (r *TCPXRouter) Logln(str string) {
}
func (r *TCPXRouter) Logf(str string) {
}

func (r *TCPXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(tcpxGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub()
		if r.IsTLS() {
			go gate.serveTLS()
		} else if r.IsUDS() {
			go gate.serveUDS()
		} else {
			go gate.serveTCP()
		}
	}
	r.WaitSubs() // gates

	r.SubsAddn(len(r.dealets) + len(r.cases))
	r.cases.walk((*tcpxCase).OnShutdown)
	r.dealets.walk(TCPXDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("tcpxRouter=%s done\n", r.Name())
	}
	r.stage.DecSub()
}

func (r *TCPXRouter) dispatch(conn *TCPXConn) {
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
}

// tcpxGate is an opening gate of TCPXRouter.
type tcpxGate struct {
	// Parent
	Gate_
	// Assocs
	router *TCPXRouter
	// States
	listener net.Listener // the real gate. set after open
}

func (g *tcpxGate) init(id int32, router *TCPXRouter) {
	g.Gate_.Init(id, router.MaxConnsPerGate())
	g.router = router
}

func (g *tcpxGate) Server() Server  { return g.router }
func (g *tcpxGate) Address() string { return g.router.Address() }
func (g *tcpxGate) IsTLS() bool     { return g.router.IsTLS() }
func (g *tcpxGate) IsUDS() bool     { return g.router.IsUDS() }

func (g *tcpxGate) Open() error {
	if g.IsUDS() {
		return g._openUnix()
	} else {
		return g._openInet()
	}
}
func (g *tcpxGate) _openUnix() error {
	address := g.Address()
	// UDS doesn't support SO_REUSEADDR or SO_REUSEPORT, so we have to remove it first.
	// This affects graceful upgrading, maybe we can implement fd transfer in the future.
	os.Remove(address)
	listener, err := net.Listen("unix", address)
	if err == nil {
		g.listener = listener.(*net.UnixListener)
	}
	return err
}
func (g *tcpxGate) _openInet() error {
	listenConfig := new(net.ListenConfig)
	listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
		// Don't use SetDeferAccept here as it assumes that clients send data first. Maybe we can make this as a config option?
		return system.SetReusePort(rawConn)
	}
	listener, err := listenConfig.Listen(context.Background(), "tcp", g.Address())
	if err == nil {
		g.listener = listener.(*net.TCPListener)
	}
	return err
}
func (g *tcpxGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serve()
}

func (g *tcpxGate) serveTLS() { // runner
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			tlsConn := tls.Server(tcpConn, g.router.TLSConfig())
			if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
				g.justClose(tlsConn)
				continue
			}
			conn := getTCPXConn(connID, g, tlsConn, nil)
			go conn.serve() // conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TLS done\n", g.id)
	}
	g.router.DecSub()
}
func (g *tcpxGate) serveUDS() { // runner
	listener := g.listener.(*net.UnixListener)
	connID := int64(0)
	for {
		unixConn, err := listener.AcceptUnix()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(unixConn)
		} else {
			rawConn, err := unixConn.SyscallConn()
			if err != nil {
				g.justClose(unixConn)
				continue
			}
			conn := getTCPXConn(connID, g, unixConn, rawConn)
			go conn.serve() // conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TCP done\n", g.id)
	}
	g.router.DecSub()
}
func (g *tcpxGate) serveTCP() { // runner
	listener := g.listener.(*net.TCPListener)
	connID := int64(0)
	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncSub()
		if g.ReachLimit() {
			g.justClose(tcpConn)
		} else {
			rawConn, err := tcpConn.SyscallConn()
			if err != nil {
				g.justClose(tcpConn)
				continue
			}
			conn := getTCPXConn(connID, g, tcpConn, rawConn)
			if DebugLevel() >= 1 {
				Printf("%+v\n", conn)
			}
			go conn.serve() // conn is put to pool in serve()
			connID++
		}
	}
	g.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TCP done\n", g.id)
	}
	g.router.DecSub()
}

func (g *tcpxGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.OnConnClosed()
}

// poolTCPXConn
var poolTCPXConn sync.Pool

func getTCPXConn(id int64, gate *tcpxGate, netConn net.Conn, rawConn syscall.RawConn) *TCPXConn {
	var tcpxConn *TCPXConn
	if x := poolTCPXConn.Get(); x == nil {
		tcpxConn = new(TCPXConn)
	} else {
		tcpxConn = x.(*TCPXConn)
	}
	tcpxConn.onGet(id, gate, netConn, rawConn)
	return tcpxConn
}
func putTCPXConn(tcpxConn *TCPXConn) {
	tcpxConn.onPut()
	poolTCPXConn.Put(tcpxConn)
}

// TCPXConn is a TCPX connection coming from TCPXRouter.
type TCPXConn struct {
	// Parent
	ServerConn_
	// Conn states (stocks)
	stockInput  [8192]byte // for c.input
	stockBuffer [256]byte  // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	router    *TCPXRouter
	gate      *tcpxGate
	netConn   net.Conn        // the connection (TCP/TLS/UDS)
	rawConn   syscall.RawConn // for syscall, only when netConn is TCP
	input     []byte          // input buffer
	region    Region          // a region-based memory pool
	closeSema atomic.Int32
	// Conn states (zeros)
}

func (c *TCPXConn) onGet(id int64, gate *tcpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id)

	c.router = gate.router
	c.gate = gate
	c.netConn = netConn
	c.rawConn = rawConn
	c.input = c.stockInput[:]
	c.region.Init()
	c.closeSema.Store(2)
}
func (c *TCPXConn) onPut() {
	c.region.Free()
	if cap(c.input) != cap(c.stockInput) {
		PutNK(c.input)
		c.input = nil
	}
	c.netConn = nil
	c.rawConn = nil
	c.gate = nil
	c.router = nil

	c.ServerConn_.OnPut()
}

func (c *TCPXConn) IsTLS() bool { return c.router.IsTLS() }
func (c *TCPXConn) IsUDS() bool { return c.router.IsUDS() }

func (c *TCPXConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.router.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *TCPXConn) serve() { // runner
	c.router.dispatch(c)
	c.closeConn()
	putTCPXConn(c)
}

func (c *TCPXConn) Recv() (p []byte, err error) { // p == nil means EOF
	// TODO: deadline
	n, err := c.netConn.Read(c.input)
	if n > 0 {
		p = c.input[:n]
	}
	if err != nil {
		c._checkClose()
	}
	return
}
func (c *TCPXConn) Send(p []byte) (err error) { // if p is nil, send EOF
	// TODO: deadline
	if p == nil {
		c.closeWrite()
		c._checkClose()
	} else {
		_, err = c.netConn.Write(p)
	}
	return
}
func (c *TCPXConn) _checkClose() {
	if c.closeSema.Add(-1) == 0 {
		c.closeConn()
	}
}

func (c *TCPXConn) closeWrite() {
	if c.router.IsTLS() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else if c.router.IsUDS() {
		c.netConn.(*net.UnixConn).CloseWrite()
	} else {
		c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TCPXConn) closeConn() {
	c.netConn.Close()
	c.gate.OnConnClosed()
}

func (c *TCPXConn) unsafeVariable(code int16, name string) (value []byte) {
	return tcpxConnVariables[code](c)
}

// tcpxConnVariables
var tcpxConnVariables = [...]func(*TCPXConn) []byte{ // keep sync with varCodes
	// TODO
	nil, // srcHost
	nil, // srcPort
	nil, // isTLS
	nil, // isUDS
	nil, // serverName
	nil, // nextProto
}

// tcpxCase
type tcpxCase struct {
	// Parent
	Component_
	// Assocs
	router  *TCPXRouter
	dealets []TCPXDealet
	// States
	general  bool
	varCode  int16
	varName  string
	patterns [][]byte
	regexps  []*regexp.Regexp
	matcher  func(kase *tcpxCase, conn *TCPXConn, value []byte) bool
}

func (c *tcpxCase) onCreate(name string, router *TCPXRouter) {
	c.MakeComp(name)
	c.router = router
}
func (c *tcpxCase) OnShutdown() {
	c.router.DecSub()
}

func (c *tcpxCase) OnConfigure() {
	if c.info == nil {
		c.general = true
		return
	}
	cond := c.info.(caseCond)
	c.varCode = cond.varCode
	c.varName = cond.varName
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
	if matcher, ok := tcpxCaseMatchers[cond.compare]; ok {
		c.matcher = matcher
	} else {
		UseExitln("unknown compare in case condition")
	}
}
func (c *tcpxCase) OnPrepare() {
}

func (c *tcpxCase) addDealet(dealet TCPXDealet) { c.dealets = append(c.dealets, dealet) }

func (c *tcpxCase) isMatch(conn *TCPXConn) bool {
	if c.general {
		return true
	}
	value := conn.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, conn, value)
}

func (c *tcpxCase) execute(conn *TCPXConn) (dealt bool) {
	for _, dealet := range c.dealets {
		if dealt := dealet.Deal(conn); dealt {
			return true
		}
	}
	return false
}

var tcpxCaseMatchers = map[string]func(kase *tcpxCase, conn *TCPXConn, value []byte) bool{
	"==": (*tcpxCase).equalMatch,
	"^=": (*tcpxCase).prefixMatch,
	"$=": (*tcpxCase).suffixMatch,
	"*=": (*tcpxCase).containMatch,
	"~=": (*tcpxCase).regexpMatch,
	"!=": (*tcpxCase).notEqualMatch,
	"!^": (*tcpxCase).notPrefixMatch,
	"!$": (*tcpxCase).notSuffixMatch,
	"!*": (*tcpxCase).notContainMatch,
	"!~": (*tcpxCase).notRegexpMatch,
}

func (c *tcpxCase) equalMatch(conn *TCPXConn, value []byte) bool { // value == patterns
	return equalMatch(value, c.patterns)
}
func (c *tcpxCase) prefixMatch(conn *TCPXConn, value []byte) bool { // value ^= patterns
	return prefixMatch(value, c.patterns)
}
func (c *tcpxCase) suffixMatch(conn *TCPXConn, value []byte) bool { // value $= patterns
	return suffixMatch(value, c.patterns)
}
func (c *tcpxCase) containMatch(conn *TCPXConn, value []byte) bool { // value *= patterns
	return containMatch(value, c.patterns)
}
func (c *tcpxCase) regexpMatch(conn *TCPXConn, value []byte) bool { // value ~= patterns
	return regexpMatch(value, c.regexps)
}
func (c *tcpxCase) notEqualMatch(conn *TCPXConn, value []byte) bool { // value != patterns
	return notEqualMatch(value, c.patterns)
}
func (c *tcpxCase) notPrefixMatch(conn *TCPXConn, value []byte) bool { // value !^ patterns
	return notPrefixMatch(value, c.patterns)
}
func (c *tcpxCase) notSuffixMatch(conn *TCPXConn, value []byte) bool { // value !$ patterns
	return notSuffixMatch(value, c.patterns)
}
func (c *tcpxCase) notContainMatch(conn *TCPXConn, value []byte) bool { // value !* patterns
	return notContainMatch(value, c.patterns)
}
func (c *tcpxCase) notRegexpMatch(conn *TCPXConn, value []byte) bool { // value !~ patterns
	return notRegexpMatch(value, c.regexps)
}

// TCPXDealet
type TCPXDealet interface {
	// Imports
	Component
	// Methods
	Deal(conn *TCPXConn) (dealt bool)
}

// TCPXDealet_
type TCPXDealet_ struct {
	// Parent
	Component_
	// States
}

// tcpxProxy passes TCPX connections to TCPX backends.
type tcpxProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage   *Stage       // current stage
	router  *TCPXRouter  // the router to which the dealet belongs
	backend *TCPXBackend // the backend to pass to
	// States
}

func (d *tcpxProxy) onCreate(name string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *tcpxProxy) OnShutdown() {
	d.router.DecSub()
}

func (d *tcpxProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpxBackend, ok := backend.(*TCPXBackend); ok {
				d.backend = tcpxBackend
			} else {
				UseExitf("incorrect backend '%s' for tcpxProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for tcpxProxy proxy")
	}
}
func (d *tcpxProxy) OnPrepare() {
	// Currently nothing.
}

func (d *tcpxProxy) Deal(conn *TCPXConn) (dealt bool) {
	// TODO
	return true
}
