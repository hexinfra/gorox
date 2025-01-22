// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// uwsgi backend implementation. See: https://uwsgi-docs.readthedocs.io/en/latest/Protocol.html

package hemi

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("uwsgiBackend", func(compName string, stage *Stage) Backend {
		b := new(uwsgiBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// uwsgiBackend
type uwsgiBackend struct {
	// Parent
	Backend_[*uwsgiNode]
	// States
}

func (b *uwsgiBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
}

func (b *uwsgiBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *uwsgiBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *uwsgiBackend) CreateNode(compName string) Node {
	node := new(uwsgiNode)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

// uwsgiNode
type uwsgiNode struct {
	// Parent
	Node_[*uwsgiBackend]
	// Mixins
	_contentSaver_ // so uwsgi responses can save their large contents in local file system.
	// States
}

func (n *uwsgiNode) onCreate(compName string, stage *Stage, backend *uwsgiBackend) {
	n.Node_.OnCreate(compName, stage, backend)
}

func (n *uwsgiNode) OnConfigure() {
	n.Node_.OnConfigure()
	n._contentSaver_.onConfigure(n, 0*time.Second, 0*time.Second, TmpDir()+"/web/backends/"+n.backend.compName+"/"+n.compName)
}
func (n *uwsgiNode) OnPrepare() {
	n.Node_.OnPrepare()
	n._contentSaver_.onPrepare(n, 0755)
}

func (n *uwsgiNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	if DebugLevel() >= 2 {
		Printf("uwsgiNode=%s done\n", n.compName)
	}
	n.backend.DecSub() // node
}

func (n *uwsgiNode) dial() (*uwsgiConn, error) {
	if DebugLevel() >= 2 {
		Printf("uwsgiNode=%s dial %s\n", n.compName, n.address)
	}
	var (
		conn *uwsgiConn
		err  error
	)
	if n.UDSMode() {
		conn, err = n._dialUDS()
	} else {
		conn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSubConns()
	return conn, err
}
func (n *uwsgiNode) _dialUDS() (*uwsgiConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("uwsgiNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getUwsgiConn(connID, n, netConn, rawConn), nil
}
func (n *uwsgiNode) _dialTCP() (*uwsgiConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("uwsgiNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getUwsgiConn(connID, n, netConn, rawConn), nil
}

// uwsgiConn
type uwsgiConn struct {
	// Assocs
	response uwsgiResponse // the uwsgi response
	request  uwsgiRequest  // the uwsgi request
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64 // the conn id
	node    *uwsgiNode
	region  Region // a region-based memory pool
	netConn net.Conn
	rawConn syscall.RawConn
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastWrite time.Time    // deadline of last write operation
	lastRead  time.Time    // deadline of last read operation
}

var poolUwsgiConn sync.Pool

func getUwsgiConn(id int64, node *uwsgiNode, netConn net.Conn, rawConn syscall.RawConn) *uwsgiConn {
	var conn *uwsgiConn
	if x := poolUwsgiConn.Get(); x == nil {
		conn = new(uwsgiConn)
		resp, req := &conn.response, &conn.request
		resp.conn = conn
		req.conn = conn
		req.response = resp
	} else {
		conn = x.(*uwsgiConn)
	}
	conn.onUse(id, node, netConn, rawConn)
	return conn
}
func putUwsgiConn(conn *uwsgiConn) {
	conn.onEnd()
	poolUwsgiConn.Put(conn)
}

func (c *uwsgiConn) onUse(id int64, node *uwsgiNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.region.Init()
	c.netConn = netConn
	c.rawConn = rawConn
	c.node = node
	c.response.onUse()
	c.request.onUse()
}
func (c *uwsgiConn) onEnd() {
	c.request.onEnd()
	c.response.onEnd()
	c.netConn = nil
	c.rawConn = nil
	c.node = nil
	c.region.Free()
}

func (c *uwsgiConn) buffer256() []byte          { return c.stockBuffer[:] }
func (c *uwsgiConn) unsafeMake(size int) []byte { return c.region.Make(size) }

// uwsgiResponse must implements the backendResponse interface.
type uwsgiResponse struct { // incoming. needs parsing
	// Assocs
	conn *uwsgiConn
}

func (r *uwsgiResponse) onUse() {
	// TODO
}
func (r *uwsgiResponse) onEnd() {
	// TODO
}

// uwsgiRequest
type uwsgiRequest struct { // outgoing. needs building
	// Assocs
	conn     *uwsgiConn
	response *uwsgiResponse
}

func (r *uwsgiRequest) onUse() {
	// TODO
}
func (r *uwsgiRequest) onEnd() {
	// TODO
}
