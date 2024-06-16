// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPS (TCP/TLS/UDS) reverse proxy and backend.

package hemi

import (
	"crypto/tls"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterTCPSDealet("tcpsProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSDealet {
		d := new(tcpsProxy)
		d.onCreate(name, stage, router)
		return d
	})
	RegisterBackend("tcpsBackend", func(name string, stage *Stage) Backend {
		b := new(TCPSBackend)
		b.onCreate(name, stage)
		return b
	})
}

// tcpsProxy passes TCPS connections to TCPS backends.
type tcpsProxy struct {
	// Parent
	TCPSDealet_
	// Assocs
	stage   *Stage       // current stage
	router  *TCPSRouter  // the router to which the dealet belongs
	backend *TCPSBackend // the backend to pass to
	// States
}

func (d *tcpsProxy) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *tcpsProxy) OnShutdown() {
	d.router.DecSub()
}

func (d *tcpsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				d.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for tcpsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for tcpsProxy proxy")
	}
}
func (d *tcpsProxy) OnPrepare() {
	// Currently nothing.
}

func (d *tcpsProxy) Deal(conn *TCPSConn) (dealt bool) {
	// TODO
	return true
}

// TCPSBackend component.
type TCPSBackend struct {
	// Parent
	Backend_[*tcpsNode]
	// States
}

func (b *TCPSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *TCPSBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *TCPSBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *TCPSBackend) CreateNode(name string) Node {
	node := new(tcpsNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *TCPSBackend) Dial() (*TConn, error) {
	node := b.nodes[b.nextIndex()]
	return node.dial()
}

// tcpsNode is a node in TCPSBackend.
type tcpsNode struct {
	// Parent
	Node_
	// Assocs
	backend *TCPSBackend
	// States
	connPool struct { // free list of conns in this node
		sync.Mutex
		head *TConn // head element
		tail *TConn // tail element
		qnty int    // size of the list
	}
}

func (n *tcpsNode) onCreate(name string, backend *TCPSBackend) {
	n.Node_.OnCreate(name)
	n.backend = backend
}

func (n *tcpsNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *tcpsNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *tcpsNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	if DebugLevel() >= 2 {
		Printf("tcpsNode=%s done\n", n.name)
	}
	n.backend.DecSub()
}

func (n *tcpsNode) dial() (*TConn, error) { // some protocols don't support or need connection reusing, just dial & tConn.close.
	if DebugLevel() >= 2 {
		Printf("tcpsNode=%s dial %s\n", n.name, n.address)
	}
	var (
		tConn *TConn
		err   error
	)
	if n.IsTLS() {
		tConn, err = n._dialTLS()
	} else if n.IsUDS() {
		tConn, err = n._dialUDS()
	} else {
		tConn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	return tConn, err
}
func (n *tcpsNode) _dialTLS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpsNode=%s dial %s OK!\n", n.name, n.address)
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
func (n *tcpsNode) _dialUDS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpsNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}
func (n *tcpsNode) _dialTCP() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpsNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}

// poolTConn
var poolTConn sync.Pool

func getTConn(id int64, node *tcpsNode, netConn net.Conn, rawConn syscall.RawConn) *TConn {
	var tConn *TConn
	if x := poolTConn.Get(); x == nil {
		tConn = new(TConn)
	} else {
		tConn = x.(*TConn)
	}
	tConn.onGet(id, node, netConn, rawConn)
	return tConn
}
func putTConn(tConn *TConn) {
	tConn.onPut()
	poolTConn.Put(tConn)
}

// TConn is a backend-side connection to tcpsNode.
type TConn struct {
	// Parent
	BackendConn_
	// Assocs
	next *TConn // the linked-list
	// Conn states (stocks)
	stockInput [8192]byte // for c.input
	// Conn states (controlled)
	// Conn states (non-zeros)
	backend *TCPSBackend
	node    *tcpsNode
	netConn net.Conn        // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn syscall.RawConn // for syscall. only usable when netConn is TCP/UDS
	input   []byte
	// Conn states (zeros)
	writeBroken atomic.Bool // write-side broken?
	readBroken  atomic.Bool // read-side broken?
}

func (c *TConn) onGet(id int64, node *tcpsNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.BackendConn_.OnGet(id, node.backend.aliveTimeout)

	c.backend = node.backend
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
	c.input = c.stockInput[:]
}
func (c *TConn) onPut() {
	c.input = nil
	c.netConn = nil
	c.rawConn = nil
	c.node = nil
	c.backend = nil

	c.writeBroken.Store(false)
	c.readBroken.Store(false)

	c.BackendConn_.OnPut()
}

func (c *TConn) IsTLS() bool { return c.node.IsTLS() }
func (c *TConn) IsUDS() bool { return c.node.IsUDS() }

func (c *TConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *TConn) TLSConn() *tls.Conn     { return c.netConn.(*tls.Conn) }
func (c *TConn) UDSConn() *net.UnixConn { return c.netConn.(*net.UnixConn) }
func (c *TConn) TCPConn() *net.TCPConn  { return c.netConn.(*net.TCPConn) }

func (c *TConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *TConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *TConn) Write(p []byte) (n int, err error)         { return c.netConn.Write(p) }
func (c *TConn) Writev(vector *net.Buffers) (int64, error) { return vector.WriteTo(c.netConn) }
func (c *TConn) Read(p []byte) (n int, err error)          { return c.netConn.Read(p) }
func (c *TConn) ReadFull(p []byte) (n int, err error)      { return io.ReadFull(c.netConn, p) }
func (c *TConn) ReadAtLeast(p []byte, min int) (n int, err error) {
	return io.ReadAtLeast(c.netConn, p, min)
}

func (c *TConn) ProxyPass(conn *TCPSConn) error {
	var (
		p   []byte
		err error
	)
	for {
		p, err = conn.Recv()
		if err != nil {
			return err
		}
		if p == nil {
			if err = c.CloseWrite(); err != nil {
				c.markWriteBroken()
				return err
			}
			return nil
		}
		if _, err = c.Write(p); err != nil {
			c.markWriteBroken()
			return err
		}
	}
}

func (c *TConn) IsBroken() bool { return c.writeBroken.Load() || c.readBroken.Load() }
func (c *TConn) MarkBroken() {
	c.markWriteBroken()
	c.markReadBroken()
}

func (c *TConn) markWriteBroken() { c.writeBroken.Store(true) }
func (c *TConn) markReadBroken()  { c.readBroken.Store(true) }

func (c *TConn) CloseWrite() error {
	if c.IsTLS() {
		return c.netConn.(*tls.Conn).CloseWrite()
	} else if c.IsUDS() {
		return c.netConn.(*net.UnixConn).CloseWrite()
	} else {
		return c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TConn) Close() error {
	netConn := c.netConn
	putTConn(c)
	return netConn.Close()
}
