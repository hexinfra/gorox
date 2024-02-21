// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC (UDP/UDS) reverse proxy.

package hemi

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quix"
)

func init() {
	RegisterQUICDealet("quicProxy", func(name string, stage *Stage, router *QUICRouter) QUICDealet {
		d := new(quicProxy)
		d.onCreate(name, stage, router)
		return d
	})
	RegisterBackend("quicBackend", func(name string, stage *Stage) Backend {
		b := new(QUICBackend)
		b.onCreate(name, stage)
		return b
	})
}

// QUICRouter
type QUICRouter struct {
	// Mixins
	router_[*QUICRouter, *quicGate, QUICDealet, *quicCase]
}

func (r *QUICRouter) onCreate(name string, stage *Stage) {
	r.router_.onCreate(name, stage, quicDealetCreators)
}
func (r *QUICRouter) OnShutdown() {
	r.router_.onShutdown()
}

func (r *QUICRouter) OnConfigure() {
	r.router_.onConfigure()
	// TODO: configure r
	r.configureSubs()
}
func (r *QUICRouter) OnPrepare() {
	r.router_.onPrepare()
	// TODO: prepare r
	r.prepareSubs()
}

func (r *QUICRouter) createCase(name string) *quicCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(quicCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}

func (r *QUICRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(quicGate)
		gate.init(id, r)
		if err := gate.Open(); err != nil {
			EnvExitln(err.Error())
		}
		r.AddGate(gate)
		r.IncSub(1)
		go gate.serve()
	}
	r.WaitSubs() // gates
	r.IncSub(len(r.dealets) + len(r.cases))
	r.shutdownSubs()
	r.WaitSubs() // dealets, cases

	if r.logger != nil {
		r.logger.Close()
	}
	if Debug() >= 2 {
		Printf("quicRouter=%s done\n", r.Name())
	}
	r.stage.SubDone()
}

func (r *QUICRouter) dispatch(conn *QUICConn) {
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
}

// QUICDealet
type QUICDealet interface {
	// Imports
	Component
	// Methods
	Deal(conn *QUICConn, stream *QUICStream) (dealt bool)
}

// QUICDealet_
type QUICDealet_ struct {
	// Mixins
	Component_
	// States
}

// quicCase
type quicCase struct {
	// Mixins
	case_[*QUICRouter, QUICDealet]
	// States
	matcher func(kase *quicCase, conn *QUICConn, value []byte) bool
}

func (c *quicCase) OnConfigure() {
	c.case_.OnConfigure()
	if c.info != nil {
		cond := c.info.(caseCond)
		if matcher, ok := quicCaseMatchers[cond.compare]; ok {
			c.matcher = matcher
		} else {
			UseExitln("unknown compare in case condition")
		}
	}
}
func (c *quicCase) OnPrepare() {
	c.case_.OnPrepare()
}

func (c *quicCase) isMatch(conn *QUICConn) bool {
	if c.general {
		return true
	}
	value := conn.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, conn, value)
}

func (c *quicCase) execute(conn *QUICConn) (dealt bool) {
	// TODO
	return false
}

var quicCaseMatchers = map[string]func(kase *quicCase, conn *QUICConn, value []byte) bool{
	"==": (*quicCase).equalMatch,
	"^=": (*quicCase).prefixMatch,
	"$=": (*quicCase).suffixMatch,
	"*=": (*quicCase).containMatch,
	"~=": (*quicCase).regexpMatch,
	"!=": (*quicCase).notEqualMatch,
	"!^": (*quicCase).notPrefixMatch,
	"!$": (*quicCase).notSuffixMatch,
	"!*": (*quicCase).notContainMatch,
	"!~": (*quicCase).notRegexpMatch,
}

func (c *quicCase) equalMatch(conn *QUICConn, value []byte) bool { // value == patterns
	return c.case_._equalMatch(value)
}
func (c *quicCase) prefixMatch(conn *QUICConn, value []byte) bool { // value ^= patterns
	return c.case_._prefixMatch(value)
}
func (c *quicCase) suffixMatch(conn *QUICConn, value []byte) bool { // value $= patterns
	return c.case_._suffixMatch(value)
}
func (c *quicCase) containMatch(conn *QUICConn, value []byte) bool { // value *= patterns
	return c.case_._containMatch(value)
}
func (c *quicCase) regexpMatch(conn *QUICConn, value []byte) bool { // value ~= patterns
	return c.case_._regexpMatch(value)
}
func (c *quicCase) notEqualMatch(conn *QUICConn, value []byte) bool { // value != patterns
	return c.case_._notEqualMatch(value)
}
func (c *quicCase) notPrefixMatch(conn *QUICConn, value []byte) bool { // value !^ patterns
	return c.case_._notPrefixMatch(value)
}
func (c *quicCase) notSuffixMatch(conn *QUICConn, value []byte) bool { // value !$ patterns
	return c.case_._notSuffixMatch(value)
}
func (c *quicCase) notContainMatch(conn *QUICConn, value []byte) bool { // value !* patterns
	return c.case_._notContainMatch(value)
}
func (c *quicCase) notRegexpMatch(conn *QUICConn, value []byte) bool { // value !~ patterns
	return c.case_._notRegexpMatch(value)
}

// quicGate is an opening gate of QUICRouter.
type quicGate struct {
	// Mixins
	Gate_
	// Assocs
	// States
	listener *quix.Listener // the real gate. set after open
}

func (g *quicGate) init(id int32, router *QUICRouter) {
	g.Gate_.Init(id, router)
}

func (g *quicGate) Open() error {
	// TODO
	// set g.listener
	return nil
}
func (g *quicGate) Shut() error {
	g.MarkShut()
	return g.listener.Close()
}

func (g *quicGate) serve() { // runner
	// TODO
	for !g.IsShut() {
		time.Sleep(time.Second)
	}
	g.server.SubDone()
}

func (g *quicGate) justClose(quixConn *quix.Conn) {
	quixConn.Close()
	g.onConnectionClosed()
}
func (g *quicGate) onConnectionClosed() {
	g.DecConns()
}

// poolQUICConn
var poolQUICConn sync.Pool

func getQUICConn(id int64, gate *quicGate, quixConn *quix.Conn) *QUICConn {
	var quicConn *QUICConn
	if x := poolQUICConn.Get(); x == nil {
		quicConn = new(QUICConn)
	} else {
		quicConn = x.(*QUICConn)
	}
	quicConn.onGet(id, gate, quixConn)
	return quicConn
}
func putQUICConn(quicConn *QUICConn) {
	quicConn.onPut()
	poolQUICConn.Put(quicConn)
}

// QUICConn is a QUIC connection coming from QUICRouter.
type QUICConn struct {
	// Mixins
	ServerConn_
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	quixConn *quix.Conn
	// Conn states (zeros)
	quicConn0
}
type quicConn0 struct { // for fast reset, entirely
}

func (c *QUICConn) onGet(id int64, gate *quicGate, quixConn *quix.Conn) {
	c.ServerConn_.OnGet(id, gate)
	c.quixConn = quixConn
}
func (c *QUICConn) onPut() {
	c.quixConn = nil
	c.quicConn0 = quicConn0{}
	c.ServerConn_.OnPut()
}

func (c *QUICConn) serve() { // runner
	router := c.Server().(*QUICRouter)
	router.dispatch(c)
	c.closeConn()
	putQUICConn(c)
}

func (c *QUICConn) closeConn() error {
	// TODO
	return nil
}

func (c *QUICConn) unsafeVariable(code int16, name string) (value []byte) {
	return quicConnVariables[code](c)
}

// quicConnVariables
var quicConnVariables = [...]func(*QUICConn) []byte{ // keep sync with varCodes in config.go
	// TODO
}

// QUICStream
type QUICStream struct {
}

func (s *QUICStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QUICStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

// quicProxy passes QUIC connections to backend QUIC server.
type quicProxy struct {
	// Mixins
	QUICDealet_
	// Assocs
	stage   *Stage // current stage
	router  *QUICRouter
	backend *QUICBackend
	// States
}

func (d *quicProxy) onCreate(name string, stage *Stage, router *QUICRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *quicProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *quicProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if quicBackend, ok := backend.(*QUICBackend); ok {
				d.backend = quicBackend
			} else {
				UseExitf("incorrect backend '%s' for quicProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for quicProxy")
	}
}
func (d *quicProxy) OnPrepare() {
}

func (d *quicProxy) Deal(conn *QUICConn, stream *QUICStream) (dealt bool) {
	// TODO
	return true
}

// QUICBackend component.
type QUICBackend struct {
	// Mixins
	Backend_[*quicNode]
	_streamHolder_
	_loadBalancer_
	// States
	health any // TODO
}

func (b *QUICBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage, b.NewNode)
	b._loadBalancer_.init()
}

func (b *QUICBackend) OnConfigure() {
	b.Backend_.OnConfigure()
	b._streamHolder_.onConfigure(b, 1000)
	b._loadBalancer_.onConfigure(b)
}
func (b *QUICBackend) OnPrepare() {
	b.Backend_.OnPrepare()
	b._streamHolder_.onPrepare(b)
	b._loadBalancer_.onPrepare(len(b.nodes))
}

func (b *QUICBackend) NewNode(id int32) *quicNode {
	node := new(quicNode)
	node.init(id, b)
	return node
}

func (b *QUICBackend) Dial() (*QConn, error) {
	// TODO
	return nil, nil
}
func (b *QUICBackend) FetchConn() (*QConn, error) {
	// TODO
	return nil, nil
}
func (b *QUICBackend) StoreConn(qConn *QConn) {
	// TODO
}

// quicNode is a node in QUICBackend.
type quicNode struct {
	// Mixins
	Node_
	// Assocs
	// States
}

func (n *quicNode) init(id int32, backend *QUICBackend) {
	n.Node_.Init(id, backend)
}

func (n *quicNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("quicNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *quicNode) dial() (*QConn, error) {
	// TODO
	return nil, nil
}
func (n *quicNode) fetchConn() (*QConn, error) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *quicNode) storeConn(qConn *QConn) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
}

func (n *quicNode) closeConn(qConn *QConn) {
	qConn.Close()
	n.SubDone()
}

// poolQConn
var poolQConn sync.Pool

func getQConn(id int64, node *quicNode, quixConn *quix.Conn) *QConn {
	var qConn *QConn
	if x := poolQConn.Get(); x == nil {
		qConn = new(QConn)
	} else {
		qConn = x.(*QConn)
	}
	qConn.onGet(id, node, quixConn)
	return qConn
}
func putQConn(qConn *QConn) {
	qConn.onPut()
	poolQConn.Put(qConn)
}

// QConn is a backend-side quic connection to quicNode.
type QConn struct {
	// Mixins
	BackendConn_
	// Conn states (non-zeros)
	quixConn   *quix.Conn
	maxStreams int32 // how many streams are allowed on this connection?
	// Conn states (zeros)
	usedStreams atomic.Int32 // how many streams has been used?
	broken      atomic.Bool  // is connection broken?
}

func (c *QConn) onGet(id int64, node *quicNode, quixConn *quix.Conn) {
	c.BackendConn_.OnGet(id, node)
	c.quixConn = quixConn
	c.maxStreams = node.Backend().(*QUICBackend).MaxStreamsPerConn()
}
func (c *QConn) onPut() {
	c.quixConn = nil
	c.usedStreams.Store(0)
	c.broken.Store(false)
	c.BackendConn_.OnPut()
}

func (c *QConn) reachLimit() bool { return c.usedStreams.Add(1) > c.maxStreams }

func (c *QConn) isBroken() bool { return c.broken.Load() }
func (c *QConn) markBroken()    { c.broken.Store(true) }

func (c *QConn) FetchStream() *QStream {
	// TODO
	return nil
}
func (c *QConn) StoreStream(stream *QStream) {
	// TODO
}

func (c *QConn) Close() error {
	quixConn := c.quixConn
	putQConn(c)
	return quixConn.Close()
}

// poolQStream
var poolQStream sync.Pool

func getQStream(conn *QConn, quixStream *quix.Stream) *QStream {
	var stream *QStream
	if x := poolQStream.Get(); x == nil {
		stream = new(QStream)
	} else {
		stream = x.(*QStream)
	}
	stream.onUse(conn, quixStream)
	return stream
}
func putQStream(stream *QStream) {
	stream.onEnd()
	poolQStream.Put(stream)
}

// QStream is a bidirectional stream of QConn.
type QStream struct {
	// TODO
	conn       *QConn
	quixStream *quix.Stream
}

func (s *QStream) onUse(conn *QConn, quixStream *quix.Stream) {
	s.conn = conn
	s.quixStream = quixStream
}
func (s *QStream) onEnd() {
	s.conn = nil
	s.quixStream = nil
}

func (s *QStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

func (s *QStream) Close() error {
	// TODO
	return nil
}
