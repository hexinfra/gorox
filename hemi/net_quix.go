// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIX (UDP/UDS) reverse proxy.

package hemi

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quic"
)

func init() {
	RegisterBackend("quixBackend", func(name string, stage *Stage) Backend {
		b := new(QUIXBackend)
		b.onCreate(name, stage)
		return b
	})
	RegisterQUIXDealet("quixProxy", func(name string, stage *Stage, router *QUIXRouter) QUIXDealet {
		d := new(quixProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// QUIXBackend component.
type QUIXBackend struct {
	// Parent
	Backend_[*quixNode]
	// Mixins
	_streamHolder_
	_loadBalancer_
	// States
	health any // TODO
}

func (b *QUIXBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage, b.NewNode)
	b._loadBalancer_.init()
}

func (b *QUIXBackend) OnConfigure() {
	b.Backend_.OnConfigure()
	b._streamHolder_.onConfigure(b, 1000)
	b._loadBalancer_.onConfigure(b)
}
func (b *QUIXBackend) OnPrepare() {
	b.Backend_.OnPrepare()
	b._streamHolder_.onPrepare(b)
	b._loadBalancer_.onPrepare(len(b.nodes))
}

func (b *QUIXBackend) NewNode(id int32) *quixNode {
	node := new(quixNode)
	node.init(id, b)
	return node
}

func (b *QUIXBackend) Dial() (*QConn, error) {
	// TODO
	return nil, nil
}
func (b *QUIXBackend) FetchConn() (*QConn, error) {
	// TODO
	return nil, nil
}
func (b *QUIXBackend) StoreConn(qConn *QConn) {
	// TODO
}

// quixNode is a node in QUIXBackend.
type quixNode struct {
	// Parent
	Node_
	// Assocs
	// States
}

func (n *quixNode) init(id int32, backend *QUIXBackend) {
	n.Node_.Init(id, backend)
}

func (n *quixNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("quixNode=%d done\n", n.id)
	}
	n.backend.DecSub()
}

func (n *quixNode) dial() (*QConn, error) {
	// TODO
	return nil, nil
}
func (n *quixNode) fetchConn() (*QConn, error) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *quixNode) storeConn(qConn *QConn) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
}

func (n *quixNode) closeConn(qConn *QConn) {
	qConn.Close()
	n.DecSub()
}

// poolQConn
var poolQConn sync.Pool

func getQConn(id int64, node *quixNode, quicConn *quic.Conn) *QConn {
	var qConn *QConn
	if x := poolQConn.Get(); x == nil {
		qConn = new(QConn)
	} else {
		qConn = x.(*QConn)
	}
	qConn.onGet(id, node, quicConn)
	return qConn
}
func putQConn(qConn *QConn) {
	qConn.onPut()
	poolQConn.Put(qConn)
}

// QConn is a backend-side quix connection to quixNode.
type QConn struct {
	// Parent
	BackendConn_
	// Conn states (non-zeros)
	quicConn   *quic.Conn
	maxStreams int32 // how many streams are allowed on this connection?
	// Conn states (zeros)
	usedStreams atomic.Int32 // how many streams has been used?
	broken      atomic.Bool  // is connection broken?
}

func (c *QConn) onGet(id int64, node *quixNode, quicConn *quic.Conn) {
	c.BackendConn_.OnGet(id, node)
	c.quicConn = quicConn
	c.maxStreams = node.Backend().(*QUIXBackend).MaxStreamsPerConn()
}
func (c *QConn) onPut() {
	c.quicConn = nil
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
	quicConn := c.quicConn
	putQConn(c)
	return quicConn.Close()
}

// poolQStream
var poolQStream sync.Pool

func getQStream(conn *QConn, quicStream *quic.Stream) *QStream {
	var stream *QStream
	if x := poolQStream.Get(); x == nil {
		stream = new(QStream)
	} else {
		stream = x.(*QStream)
	}
	stream.onUse(conn, quicStream)
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
	quicStream *quic.Stream
}

func (s *QStream) onUse(conn *QConn, quicStream *quic.Stream) {
	s.conn = conn
	s.quicStream = quicStream
}
func (s *QStream) onEnd() {
	s.conn = nil
	s.quicStream = nil
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

// QUIXRouter
type QUIXRouter struct {
	// Parent
	router_[*QUIXRouter, *quixGate, QUIXDealet, *quixCase]
}

func (r *QUIXRouter) onCreate(name string, stage *Stage) {
	r.router_.onCreate(name, stage, quixDealetCreators)
}
func (r *QUIXRouter) OnShutdown() {
	r.router_.onShutdown()
}

func (r *QUIXRouter) OnConfigure() {
	r.router_.onConfigure()
	// TODO: configure r
	r.configureSubs()
}
func (r *QUIXRouter) OnPrepare() {
	r.router_.onPrepare()
	// TODO: prepare r
	r.prepareSubs()
}

func (r *QUIXRouter) createCase(name string) *quixCase {
	if r.hasCase(name) {
		UseExitln("conflicting case with a same name")
	}
	kase := new(quixCase)
	kase.onCreate(name, r)
	kase.setShell(kase)
	r.cases = append(r.cases, kase)
	return kase
}

func (r *QUIXRouter) Serve() { // runner
	for id := int32(0); id < r.numGates; id++ {
		gate := new(quixGate)
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
		Printf("quixRouter=%s done\n", r.Name())
	}
	r.stage.DecSub()
}

func (r *QUIXRouter) dispatch(conn *QUIXConn) {
	for _, kase := range r.cases {
		if !kase.isMatch(conn) {
			continue
		}
		if dealt := kase.execute(conn); dealt {
			break
		}
	}
}

// QUIXDealet
type QUIXDealet interface {
	// Imports
	Component
	// Methods
	Deal(conn *QUIXConn, stream *QUIXStream) (dealt bool)
}

// QUIXDealet_
type QUIXDealet_ struct {
	// Parent
	Component_
	// States
}

// quixCase
type quixCase struct {
	// Parent
	case_[*QUIXRouter, QUIXDealet]
	// States
	matcher func(kase *quixCase, conn *QUIXConn, value []byte) bool
}

func (c *quixCase) OnConfigure() {
	c.case_.OnConfigure()
	if c.info != nil {
		cond := c.info.(caseCond)
		if matcher, ok := quixCaseMatchers[cond.compare]; ok {
			c.matcher = matcher
		} else {
			UseExitln("unknown compare in case condition")
		}
	}
}
func (c *quixCase) OnPrepare() {
	c.case_.OnPrepare()
}

func (c *quixCase) isMatch(conn *QUIXConn) bool {
	if c.general {
		return true
	}
	value := conn.unsafeVariable(c.varCode, c.varName)
	return c.matcher(c, conn, value)
}

func (c *quixCase) execute(conn *QUIXConn) (dealt bool) {
	// TODO
	return false
}

var quixCaseMatchers = map[string]func(kase *quixCase, conn *QUIXConn, value []byte) bool{
	"==": (*quixCase).equalMatch,
	"^=": (*quixCase).prefixMatch,
	"$=": (*quixCase).suffixMatch,
	"*=": (*quixCase).containMatch,
	"~=": (*quixCase).regexpMatch,
	"!=": (*quixCase).notEqualMatch,
	"!^": (*quixCase).notPrefixMatch,
	"!$": (*quixCase).notSuffixMatch,
	"!*": (*quixCase).notContainMatch,
	"!~": (*quixCase).notRegexpMatch,
}

func (c *quixCase) equalMatch(conn *QUIXConn, value []byte) bool { // value == patterns
	return c.case_._equalMatch(value)
}
func (c *quixCase) prefixMatch(conn *QUIXConn, value []byte) bool { // value ^= patterns
	return c.case_._prefixMatch(value)
}
func (c *quixCase) suffixMatch(conn *QUIXConn, value []byte) bool { // value $= patterns
	return c.case_._suffixMatch(value)
}
func (c *quixCase) containMatch(conn *QUIXConn, value []byte) bool { // value *= patterns
	return c.case_._containMatch(value)
}
func (c *quixCase) regexpMatch(conn *QUIXConn, value []byte) bool { // value ~= patterns
	return c.case_._regexpMatch(value)
}
func (c *quixCase) notEqualMatch(conn *QUIXConn, value []byte) bool { // value != patterns
	return c.case_._notEqualMatch(value)
}
func (c *quixCase) notPrefixMatch(conn *QUIXConn, value []byte) bool { // value !^ patterns
	return c.case_._notPrefixMatch(value)
}
func (c *quixCase) notSuffixMatch(conn *QUIXConn, value []byte) bool { // value !$ patterns
	return c.case_._notSuffixMatch(value)
}
func (c *quixCase) notContainMatch(conn *QUIXConn, value []byte) bool { // value !* patterns
	return c.case_._notContainMatch(value)
}
func (c *quixCase) notRegexpMatch(conn *QUIXConn, value []byte) bool { // value !~ patterns
	return c.case_._notRegexpMatch(value)
}

// quixGate is an opening gate of QUIXRouter.
type quixGate struct {
	// Parent
	Gate_
	// Assocs
	// States
	listener *quic.Listener // the real gate. set after open
}

func (g *quixGate) init(id int32, router *QUIXRouter) {
	g.Gate_.Init(id, router)
}

func (g *quixGate) Open() error {
	// TODO
	// set g.listener
	return nil
}
func (g *quixGate) Shut() error {
	g.MarkShut()
	return g.listener.Close()
}

func (g *quixGate) serve() { // runner
	// TODO
	for !g.IsShut() {
		time.Sleep(time.Second)
	}
	g.server.DecSub()
}

func (g *quixGate) justClose(quicConn *quic.Conn) {
	quicConn.Close()
	g.onConnectionClosed()
}
func (g *quixGate) onConnectionClosed() {
	g.DecConns()
}

// poolQUIXConn
var poolQUIXConn sync.Pool

func getQUIXConn(id int64, gate *quixGate, quicConn *quic.Conn) *QUIXConn {
	var quixConn *QUIXConn
	if x := poolQUIXConn.Get(); x == nil {
		quixConn = new(QUIXConn)
	} else {
		quixConn = x.(*QUIXConn)
	}
	quixConn.onGet(id, gate, quicConn)
	return quixConn
}
func putQUIXConn(quixConn *QUIXConn) {
	quixConn.onPut()
	poolQUIXConn.Put(quixConn)
}

// QUIXConn is a QUIX connection coming from QUIXRouter.
type QUIXConn struct {
	// Parent
	ServerConn_
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	quicConn *quic.Conn
	// Conn states (zeros)
	quixConn0
}
type quixConn0 struct { // for fast reset, entirely
}

func (c *QUIXConn) onGet(id int64, gate *quixGate, quicConn *quic.Conn) {
	c.ServerConn_.OnGet(id, gate)
	c.quicConn = quicConn
}
func (c *QUIXConn) onPut() {
	c.quicConn = nil
	c.quixConn0 = quixConn0{}
	c.ServerConn_.OnPut()
}

func (c *QUIXConn) serve() { // runner
	router := c.Server().(*QUIXRouter)
	router.dispatch(c)
	c.closeConn()
	putQUIXConn(c)
}

func (c *QUIXConn) closeConn() error {
	// TODO
	return nil
}

func (c *QUIXConn) unsafeVariable(code int16, name string) (value []byte) {
	return quixConnVariables[code](c)
}

// quixConnVariables
var quixConnVariables = [...]func(*QUIXConn) []byte{ // keep sync with varCodes in config.go
	// TODO
}

// QUIXStream
type QUIXStream struct {
	quicStream *quic.Stream
}

func (s *QUIXStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QUIXStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

// quixProxy passes QUIX connections to backend QUIX server.
type quixProxy struct {
	// Parent
	QUIXDealet_
	// Assocs
	stage   *Stage // current stage
	router  *QUIXRouter
	backend *QUIXBackend
	// States
}

func (d *quixProxy) onCreate(name string, stage *Stage, router *QUIXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *quixProxy) OnShutdown() {
	d.router.DecSub()
}

func (d *quixProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if quixBackend, ok := backend.(*QUIXBackend); ok {
				d.backend = quixBackend
			} else {
				UseExitf("incorrect backend '%s' for quixProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for quixProxy")
	}
}
func (d *quixProxy) OnPrepare() {
}

func (d *quixProxy) Deal(conn *QUIXConn, stream *QUIXStream) (dealt bool) {
	// TODO
	return true
}
