// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// uwsgi backend implementation. See: https://uwsgi-docs.readthedocs.io/en/latest/Protocol.html

package hemi

import (
	"io"
	"net"
	"sync"
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

	b.ConfigureNodes()
}
func (b *uwsgiBackend) OnPrepare() {
	b.Backend_.OnPrepare()

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
	return getUWSGIConn(connID, n, netConn, rawConn), nil
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
	return getUWSGIConn(connID, n, netConn, rawConn), nil
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
	id      int64      // the conn id
	node    *uwsgiNode // the node to which the connection belongs
	region  Region     // a region-based memory pool
	netConn net.Conn
	rawConn syscall.RawConn
	// Conn states (zeros)
	lastWrite time.Time // deadline of last write operation
	lastRead  time.Time // deadline of last read operation
}

var poolUWSGIConn sync.Pool

func getUWSGIConn(id int64, node *uwsgiNode, netConn net.Conn, rawConn syscall.RawConn) *uwsgiConn {
	var conn *uwsgiConn
	if x := poolUWSGIConn.Get(); x == nil {
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
func putUWSGIConn(conn *uwsgiConn) {
	conn.onEnd()
	poolUWSGIConn.Put(conn)
}

func (c *uwsgiConn) onUse(id int64, node *uwsgiNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.node = node
	c.region.Init()
	c.netConn = netConn
	c.rawConn = rawConn
	c.response.onUse()
	c.request.onUse()
}
func (c *uwsgiConn) onEnd() {
	c.request.onEnd()
	c.response.onEnd()
	c.node = nil
	c.region.Free()
	c.netConn = nil
	c.rawConn = nil
}

func (c *uwsgiConn) MakeTempName(dst []byte, unixTime int64) int {
	return makeTempName(dst, c.node.Stage().ID(), c.id, unixTime, 0)
}

func (c *uwsgiConn) setReadDeadline() error {
	if deadline := time.Now().Add(c.node.readTimeout); deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *uwsgiConn) setWriteDeadline() error {
	if deadline := time.Now().Add(c.node.writeTimeout); deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *uwsgiConn) read(dst []byte) (int, error) { return c.netConn.Read(dst) }
func (c *uwsgiConn) readAtLeast(dst []byte, min int) (int, error) {
	return io.ReadAtLeast(c.netConn, dst, min)
}
func (c *uwsgiConn) write(src []byte) (int, error)             { return c.netConn.Write(src) }
func (c *uwsgiConn) writev(srcVec *net.Buffers) (int64, error) { return srcVec.WriteTo(c.netConn) }

func (c *uwsgiConn) buffer256() []byte          { return c.stockBuffer[:] }
func (c *uwsgiConn) unsafeMake(size int) []byte { return c.region.Make(size) }

func (c *uwsgiConn) Close() error {
	netConn := c.netConn
	putUWSGIConn(c)
	return netConn.Close()
}

// uwsgiResponse must implements the backendResponse interface.
type uwsgiResponse struct { // incoming. needs parsing
	// Assocs
	conn *uwsgiConn
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	headResult int16 // result of receiving response head. values are same as http status for convenience
	bodyResult int16 // result of receiving response body. values are same as http status for convenience
	// Conn states (zeros)
	_uwsgiResponse0 // all values in this struct must be zero by default!
}
type _uwsgiResponse0 struct { // for fast reset, entirely
}

func (r *uwsgiResponse) onUse() {
	// TODO
}
func (r *uwsgiResponse) onEnd() {
	// TODO
	r._uwsgiResponse0 = _uwsgiResponse0{}
}

func (r *uwsgiResponse) reuse() {
	r.onEnd()
	r.onUse()
}

func (r *uwsgiResponse) KeepAlive() int8 { return -1 } // same as "no connection header". TODO: confirm this

func (r *uwsgiResponse) HeadResult() int16 { return r.headResult }
func (r *uwsgiResponse) BodyResult() int16 { return r.bodyResult }

// uwsgiRequest
type uwsgiRequest struct { // outgoing. needs building
	// Assocs
	conn     *uwsgiConn
	response *uwsgiResponse
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
	_uwsgiRequest0 // all values in this struct must be zero by default!
}
type _uwsgiRequest0 struct { // for fast reset, entirely
}

func (r *uwsgiRequest) onUse() {
	// TODO
}
func (r *uwsgiRequest) onEnd() {
	// TODO
	r._uwsgiRequest0 = _uwsgiRequest0{}
}
