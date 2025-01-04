// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPX (TCP/TLS/UDS) router, reverse proxy, and backend implementation. See RFC 9293.

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

//////////////////////////////////////// TCPX router implementation ////////////////////////////////////////

// TCPXRouter
type TCPXRouter struct {
	// Parent
	Server_[*tcpxGate]
	// Assocs
	dealets compDict[TCPXDealet] // defined dealets. indexed by name
	cases   compList[*tcpxCase]  // defined cases. the order must be kept, so we use list. TODO: use ordered map?
	// States
	accessLog *LogConfig // ...
	logger    *Logger    // router access logger
}

func (r *TCPXRouter) onCreate(name string, stage *Stage) {
	r.Server_.OnCreate(name, stage)
	r.dealets = make(compDict[TCPXDealet])
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
		//r.logger = NewLogger(r.accessLog)
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

func (r *TCPXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(tcpxGate)
		gate.onNew(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub() // gate
		if r.IsUDS() {
			go gate.serveUDS()
		} else if r.IsTLS() {
			go gate.serveTLS()
		} else {
			go gate.serveTCP()
		}
	}
	r.WaitSubs() // gates

	r.IncSubs(len(r.dealets) + len(r.cases))
	r.cases.walk((*tcpxCase).OnShutdown)
	r.dealets.walk(TCPXDealet.OnShutdown)
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("tcpxRouter=%s done\n", r.Name())
	}
	r.stage.DecSub() // router
}

func (r *TCPXRouter) Log(str string) {
	if r.logger != nil {
		r.logger.Log(str)
	}
}
func (r *TCPXRouter) Logln(str string) {
	if r.logger != nil {
		r.logger.Logln(str)
	}
}
func (r *TCPXRouter) Logf(format string, args ...any) {
	if r.logger != nil {
		r.logger.Logf(format, args...)
	}
}

func (r *TCPXRouter) serveConn(conn *TCPXConn) { // runner
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
	putTCPXConn(conn)
}

// tcpxGate is an opening gate of TCPXRouter.
type tcpxGate struct {
	// Parent
	Gate_
	// Assocs
	router *TCPXRouter
	// States
	maxActives int32        // max concurrent conns allowed for this gate
	curActives atomic.Int32 // TODO: false sharing
	listener   net.Listener // the real gate. set after open
}

func (g *tcpxGate) onNew(id int32, router *TCPXRouter) {
	g.Gate_.OnNew(id)
	g.router = router
	g.maxActives = router.MaxConnsPerGate()
	g.curActives.Store(0)
}

func (g *tcpxGate) DecActives() int32             { return g.curActives.Add(-1) }
func (g *tcpxGate) IncActives() int32             { return g.curActives.Add(1) }
func (g *tcpxGate) ReachLimit(actives int32) bool { return actives > g.maxActives }

func (g *tcpxGate) Server() Server  { return g.router }
func (g *tcpxGate) Address() string { return g.router.Address() }
func (g *tcpxGate) IsUDS() bool     { return g.router.IsUDS() }
func (g *tcpxGate) IsTLS() bool     { return g.router.IsTLS() }

func (g *tcpxGate) Open() error {
	var (
		listener net.Listener
		err      error
	)
	if g.IsUDS() {
		address := g.Address()
		// UDS doesn't support SO_REUSEADDR or SO_REUSEPORT, so we have to remove it first.
		// This affects graceful upgrading, maybe we can implement fd transfer in the future.
		os.Remove(address)
		if listener, err = net.Listen("unix", address); err == nil {
			g.listener = listener.(*net.UnixListener)
		}
	} else {
		listenConfig := new(net.ListenConfig)
		listenConfig.Control = func(network string, address string, rawConn syscall.RawConn) error {
			// Don't use SetDeferAccept here as it assumes that clients send data first. Maybe we can make this as a config option?
			return system.SetReusePort(rawConn)
		}
		if listener, err = listenConfig.Listen(context.Background(), "tcp", g.Address()); err == nil {
			g.listener = listener.(*net.TCPListener)
		}
	}
	return err
}
func (g *tcpxGate) Shut() error {
	g.MarkShut()
	return g.listener.Close() // breaks serveXXX()
}

func (g *tcpxGate) serveUDS() { // runner
	listener := g.listener.(*net.UnixListener)
	connID := int64(0)
	for {
		udsConn, err := listener.AcceptUnix()
		if err != nil {
			if g.IsShut() {
				break
			} else {
				continue
			}
		}
		g.IncConn()
		if actives := g.IncActives(); g.ReachLimit(actives) {
			g.router.Logf("tcpxGate=%d: too many UDS connections!\n", g.id)
			g.justClose(udsConn)
			continue
		}
		rawConn, err := udsConn.SyscallConn()
		if err != nil {
			g.justClose(udsConn)
			continue
		}
		conn := getTCPXConn(connID, g, udsConn, rawConn)
		go g.router.serveConn(conn) // conn is put to pool in serveConn()
		connID++
	}
	g.WaitConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TCP done\n", g.id)
	}
	g.router.DecSub() // gate
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
		g.IncConn()
		if actives := g.IncActives(); g.ReachLimit(actives) {
			g.router.Logf("tcpxGate=%d: too many TLS connections!\n", g.id)
			g.justClose(tcpConn)
			continue
		}
		tlsConn := tls.Server(tcpConn, g.router.TLSConfig())
		// TODO: configure timeout
		if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
			g.justClose(tlsConn)
			continue
		}
		conn := getTCPXConn(connID, g, tlsConn, nil)
		go g.router.serveConn(conn) // conn is put to pool in serveConn()
		connID++
	}
	g.WaitConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TLS done\n", g.id)
	}
	g.router.DecSub() // gate
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
		g.IncConn()
		if actives := g.IncActives(); g.ReachLimit(actives) {
			g.router.Logf("tcpxGate=%d: too many TCP connections!\n", g.id)
			g.justClose(tcpConn)
			continue
		}
		rawConn, err := tcpConn.SyscallConn()
		if err != nil {
			g.justClose(tcpConn)
			continue
		}
		conn := getTCPXConn(connID, g, tcpConn, rawConn)
		if DebugLevel() >= 2 {
			Printf("%+v\n", conn)
		}
		go g.router.serveConn(conn) // conn is put to pool in serveConn()
		connID++
	}
	g.WaitConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxGate=%d TCP done\n", g.id)
	}
	g.router.DecSub() // gate
}

func (g *tcpxGate) justClose(netConn net.Conn) {
	netConn.Close()
	g.DecActives()
	g.DecConn()
}

// TCPXConn is a TCPX connection coming from TCPXRouter.
type TCPXConn struct {
	// Parent
	tcpxConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	gate *tcpxGate
	// Conn states (zeros)
}

var poolTCPXConn sync.Pool

func getTCPXConn(id int64, gate *tcpxGate, netConn net.Conn, rawConn syscall.RawConn) *TCPXConn {
	var conn *TCPXConn
	if x := poolTCPXConn.Get(); x == nil {
		conn = new(TCPXConn)
	} else {
		conn = x.(*TCPXConn)
	}
	conn.onGet(id, gate, netConn, rawConn)
	return conn
}
func putTCPXConn(conn *TCPXConn) {
	conn.onPut()
	poolTCPXConn.Put(conn)
}

func (c *TCPXConn) onGet(id int64, gate *tcpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	router := gate.router
	c.tcpxConn_.onGet(id, router.Stage().ID(), netConn, rawConn, gate.IsUDS(), gate.IsTLS(), router.ReadTimeout(), router.WriteTimeout())

	c.gate = gate
}
func (c *TCPXConn) onPut() {
	c.gate = nil

	c.tcpxConn_.onPut()
}

func (c *TCPXConn) CloseRead() {
	c._checkClose()
}
func (c *TCPXConn) CloseWrite() {
	if gate := c.gate; gate.IsUDS() {
		c.netConn.(*net.UnixConn).CloseWrite()
	} else if gate.IsTLS() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else {
		c.netConn.(*net.TCPConn).CloseWrite()
	}
	c._checkClose()
}
func (c *TCPXConn) _checkClose() {
	if c.closeSema.Add(-1) == 0 {
		c.Close()
	}
}
func (c *TCPXConn) Close() {
	c.gate.justClose(c.netConn)
}

func (c *TCPXConn) unsafeVariable(code int16, name string) (value []byte) {
	return tcpxConnVariables[code](c)
}

// tcpxConnVariables
var tcpxConnVariables = [...]func(*TCPXConn) []byte{ // keep sync with varCodes
	// TODO
	0: nil, // srcHost
	1: nil, // srcPort
	2: nil, // isUDS
	3: nil, // isTLS
	4: nil, // serverName
	5: nil, // nextProto
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
	c.router.DecSub() // case
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

//////////////////////////////////////// TCPX reverse proxy implementation ////////////////////////////////////////

func init() {
	RegisterTCPXDealet("tcpxProxy", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(tcpxProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// tcpxProxy dealet passes TCPX connections to TCPX backends.
type tcpxProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage   *Stage       // current stage
	router  *TCPXRouter  // the router to which the dealet belongs
	backend *TCPXBackend // the backend to pass to
	// States
	TCPXProxyConfig // embeded
}

func (d *tcpxProxy) onCreate(name string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *tcpxProxy) OnShutdown() {
	d.router.DecSub() // dealet
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
	TCPXReverseProxy(conn, d.backend, &d.TCPXProxyConfig)
	return true
}

// TCPXProxyConfig
type TCPXProxyConfig struct {
	// Inbound
	// Outbound
}

// TCPXReverseProxy
func TCPXReverseProxy(conn *TCPXConn, backend *TCPXBackend, proxyConfig *TCPXProxyConfig) {
	backConn, backErr := backend.Dial()
	if backErr != nil {
		conn.Close()
		return
	}
	inboundOver := make(chan struct{}, 1)
	// Inbound
	go func() {
		var (
			payload []byte
			err     error
			backErr error
		)
		for {
			if err = conn.SetReadDeadline(); err == nil {
				if payload, err = conn.Recv(); len(payload) > 0 {
					if backErr = backConn.SetWriteDeadline(); backErr == nil {
						backErr = backConn.Send(payload)
					}
				}
			}
			if err != nil || backErr != nil {
				conn.CloseRead()
				backConn.CloseWrite()
				break
			}
		}
		inboundOver <- struct{}{}
	}()
	// Outbound
	var (
		payload []byte
		err     error
	)
	for {
		if backErr = backConn.SetReadDeadline(); backErr == nil {
			if payload, backErr = backConn.Recv(); len(payload) > 0 {
				if err = conn.SetWriteDeadline(); err == nil {
					err = conn.Send(payload)
				}
			}
		}
		if backErr != nil || err != nil {
			backConn.CloseRead()
			conn.CloseWrite()
			break
		}
	}
	<-inboundOver
}

//////////////////////////////////////// TCPX backend implementation ////////////////////////////////////////

func init() {
	RegisterBackend("tcpxBackend", func(name string, stage *Stage) Backend {
		b := new(TCPXBackend)
		b.onCreate(name, stage)
		return b
	})
}

// TCPXBackend component.
type TCPXBackend struct {
	// Parent
	Backend_[*tcpxNode]
	// States
}

func (b *TCPXBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *TCPXBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *TCPXBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *TCPXBackend) CreateNode(name string) Node {
	node := new(tcpxNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *TCPXBackend) Dial() (*TConn, error) {
	node := b.nodes[b.nodeIndexGet()]
	return node.dial()
}

// tcpxNode is a node in TCPXBackend.
type tcpxNode struct {
	// Parent
	Node_
	// Assocs
	backend *TCPXBackend
	// States
}

func (n *tcpxNode) onCreate(name string, backend *TCPXBackend) {
	n.Node_.OnCreate(name)
	n.backend = backend
}

func (n *tcpxNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *tcpxNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *tcpxNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	n.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s done\n", n.name)
	}
	n.backend.DecSub() // node
}

func (n *tcpxNode) dial() (*TConn, error) {
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s\n", n.name, n.address)
	}
	var (
		tConn *TConn
		err   error
	)
	if n.IsUDS() {
		tConn, err = n._dialUDS()
	} else if n.IsTLS() {
		tConn, err = n._dialTLS()
	} else {
		tConn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSub() // conn
	return tConn, err
}
func (n *tcpxNode) _dialUDS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}
func (n *tcpxNode) _dialTLS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	tlsConn := tls.Client(netConn, n.tlsConfig)
	if err := tlsConn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		tlsConn.Close()
		return nil, err
	}
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, err
	}
	return getTConn(connID, n, tlsConn, nil), nil
}
func (n *tcpxNode) _dialTCP() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}

// TConn is a backend-side connection to tcpxNode.
type TConn struct {
	// Parent
	tcpxConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node *tcpxNode
	// Conn states (zeros)
}

var poolTConn sync.Pool

func getTConn(id int64, node *tcpxNode, netConn net.Conn, rawConn syscall.RawConn) *TConn {
	var conn *TConn
	if x := poolTConn.Get(); x == nil {
		conn = new(TConn)
	} else {
		conn = x.(*TConn)
	}
	conn.onGet(id, node, netConn, rawConn)
	return conn
}
func putTConn(conn *TConn) {
	conn.onPut()
	poolTConn.Put(conn)
}

func (c *TConn) onGet(id int64, node *tcpxNode, netConn net.Conn, rawConn syscall.RawConn) {
	backend := node.backend
	c.tcpxConn_.onGet(id, backend.Stage().ID(), netConn, rawConn, node.IsUDS(), node.IsTLS(), backend.ReadTimeout(), backend.WriteTimeout())

	c.node = node
}
func (c *TConn) onPut() {
	c.node = nil

	c.tcpxConn_.onPut()
}

func (c *TConn) CloseWrite() {
	if node := c.node; node.IsUDS() {
		c.netConn.(*net.UnixConn).CloseWrite()
	} else if node.IsTLS() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else {
		c.netConn.(*net.TCPConn).CloseWrite()
	}
	c._checkClose()
}
func (c *TConn) CloseRead() {
	c._checkClose()
}
func (c *TConn) _checkClose() {
	if c.closeSema.Add(-1) == 0 {
		c.Close()
	}
}
func (c *TConn) Close() error {
	c.node.DecSub() // conn
	netConn := c.netConn
	putTConn(c)
	return netConn.Close()
}

//////////////////////////////////////// TCPX in/out implementation ////////////////////////////////////////

// tcpxConn
type tcpxConn interface {
}

// tcpxConn_
type tcpxConn_ struct {
	// Conn states (stocks)
	stockInput  [8192]byte // for c.input
	stockBuffer [256]byte  // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	id           int64 // the conn id
	stageID      int32
	udsMode      bool
	tlsMode      bool
	readTimeout  time.Duration   // for convenience
	writeTimeout time.Duration   // for convenience
	netConn      net.Conn        // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn      syscall.RawConn // for syscall, only usable when netConn is TCP/UDS
	input        []byte          // input buffer
	region       Region          // a region-based memory pool
	closeSema    atomic.Int32    // controls read/write close
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastRead  time.Time    // deadline of last read operation
	lastWrite time.Time    // deadline of last write operation
}

func (c *tcpxConn_) onGet(id int64, stageID int32, netConn net.Conn, rawConn syscall.RawConn, udsMode bool, tlsMode bool, readTimeout time.Duration, writeTimeout time.Duration) {
	c.id = id
	c.stageID = stageID
	c.netConn = netConn
	c.rawConn = rawConn
	c.udsMode = udsMode
	c.tlsMode = tlsMode
	c.readTimeout = readTimeout
	c.writeTimeout = writeTimeout
	c.input = c.stockInput[:]
	c.region.Init()
	c.closeSema.Store(2)
}
func (c *tcpxConn_) onPut() {
	c.region.Free()
	if cap(c.input) != cap(c.stockInput) {
		PutNK(c.input)
	}
	c.input = nil
	c.netConn = nil
	c.rawConn = nil

	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
}

func (c *tcpxConn_) IsUDS() bool { return c.udsMode }
func (c *tcpxConn_) IsTLS() bool { return c.tlsMode }

func (c *tcpxConn_) MakeTempName(to []byte, unixTime int64) int {
	return makeTempName(to, c.stageID, c.id, unixTime, c.counter.Add(1))
}

func (c *tcpxConn_) SetReadDeadline() error {
	deadline := time.Now().Add(c.readTimeout)
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *tcpxConn_) SetWriteDeadline() error {
	deadline := time.Now().Add(c.writeTimeout)
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *tcpxConn_) Recv() (p []byte, err error) {
	n, err := c.netConn.Read(c.input)
	p = c.input[:n]
	return
}
func (c *tcpxConn_) Send(p []byte) (err error) {
	_, err = c.netConn.Write(p)
	return
}
