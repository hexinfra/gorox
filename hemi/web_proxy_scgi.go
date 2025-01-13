// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// SCGI reverse proxy (a.k.a. gateway) and backend implementation. See: https://python.ca/scgi/protocol.txt

// HTTP trailers: not supported
// Persistent connection: not supported
// Vague request content: not supported, proxies MUST send sized requests
// Vague response content: supported

// SCGI protocol doesn't define the format of its response. Seems it follows the format of CGI response.

package hemi

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

//////////////////////////////////////// SCGI reverse proxy implementation ////////////////////////////////////////

func init() {
	RegisterHandlet("scgiProxy", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(scgiProxy)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// scgiProxy handlet passes http requests to SCGI backends and caches responses.
type scgiProxy struct {
	// Parent
	Handlet_
	// Assocs
	stage   *Stage       // current stage
	webapp  *Webapp      // the webapp to which the proxy belongs
	backend *scgiBackend // the backend to pass to
	cacher  Cacher       // the cacher which is used by this proxy
	// States
	WebExchanProxyConfig // embeded
}

func (h *scgiProxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *scgiProxy) OnShutdown() {
	h.webapp.DecSub() // handlet
}

func (h *scgiProxy) OnConfigure() {
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if scgiBackend, ok := backend.(*scgiBackend); ok {
				h.backend = scgiBackend
			} else {
				UseExitf("incorrect backend '%s' for scgiProxy, must be scgiBackend\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for scgiProxy")
	}

	// withCacher
	if v, ok := h.Find("withCacher"); ok {
		if name, ok := v.String(); ok && name != "" {
			if cacher := h.stage.Cacher(name); cacher == nil {
				UseExitf("unknown cacher: '%s'\n", name)
			} else {
				h.cacher = cacher
			}
		} else {
			UseExitln("invalid withCacher")
		}
	}

	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.BufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.BufferServerContent, true)
}
func (h *scgiProxy) OnPrepare() {
}

func (h *scgiProxy) IsProxy() bool { return true }
func (h *scgiProxy) IsCache() bool { return h.cacher != nil }

func (h *scgiProxy) Handle(httpReq Request, httpResp Response) (handled bool) {
	// TODO: implementation
	httpResp.Send("SCGI")
	return true
}

//////////////////////////////////////// SCGI backend implementation ////////////////////////////////////////

func init() {
	RegisterBackend("scgiBackend", func(name string, stage *Stage) Backend {
		b := new(scgiBackend)
		b.onCreate(name, stage)
		return b
	})
}

// scgiBackend
type scgiBackend struct {
	// Parent
	Backend_[*scgiNode]
	// States
}

func (b *scgiBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *scgiBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *scgiBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *scgiBackend) CreateNode(name string) Node {
	node := new(scgiNode)
	node.onCreate(name, b.stage, b)
	b.AddNode(node)
	return node
}

// scgiNode
type scgiNode struct {
	// Parent
	Node_[*scgiBackend]
	// Mixins
	_contentSaver_ // so responses can save their large contents in local file system.
	// States
}

func (n *scgiNode) onCreate(name string, stage *Stage, backend *scgiBackend) {
	n.Node_.OnCreate(name, stage, backend)
}

func (n *scgiNode) OnConfigure() {
	n.Node_.OnConfigure()
	n._contentSaver_.onConfigure(n, TmpDir()+"/web/backends/"+n.backend.name+"/"+n.name, 0*time.Second, 0*time.Second)
}
func (n *scgiNode) OnPrepare() {
	n.Node_.OnPrepare()
	n._contentSaver_.onPrepare(n, 0755)
}

func (n *scgiNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s done\n", n.name)
	}
	n.backend.DecSub() // node
}

func (n *scgiNode) dial() (*scgiConn, error) {
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s dial %s\n", n.name, n.address)
	}
	var (
		conn *scgiConn
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
	n.IncSub() // conn
	return conn, err
}
func (n *scgiNode) _dialUDS() (*scgiConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getSCGIConn(connID, n, netConn, rawConn), nil
}
func (n *scgiNode) _dialTCP() (*scgiConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getSCGIConn(connID, n, netConn, rawConn), nil
}

// scgiConn
type scgiConn struct {
	// Assocs
	request  scgiRequest  // the scgi request
	response scgiResponse // the scgi response
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64 // the conn id
	node    *scgiNode
	region  Region // a region-based memory pool
	netConn net.Conn
	rawConn syscall.RawConn
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastWrite time.Time    // deadline of last write operation
	lastRead  time.Time    // deadline of last read operation
}

var poolSCGIConn sync.Pool

func getSCGIConn(id int64, node *scgiNode, netConn net.Conn, rawConn syscall.RawConn) *scgiConn {
	var conn *scgiConn
	if x := poolSCGIConn.Get(); x == nil {
		conn = new(scgiConn)
		req, resp := &conn.request, &conn.response
		req.conn = conn
		req.response = resp
		resp.conn = conn
	} else {
		conn = x.(*scgiConn)
	}
	conn.onUse(id, node, netConn, rawConn)
	return conn
}
func putSCGIConn(conn *scgiConn) {
	conn.onEnd()
	poolSCGIConn.Put(conn)
}

func (c *scgiConn) onUse(id int64, node *scgiNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.region.Init()
	c.netConn = netConn
	c.rawConn = rawConn
	c.node = node
	c.request.onUse()
	c.response.onUse()
}
func (c *scgiConn) onEnd() {
	c.request.onEnd()
	c.response.onEnd()
	c.netConn = nil
	c.rawConn = nil
	c.node = nil
	c.region.Free()
}

func (c *scgiConn) buffer256() []byte          { return c.stockBuffer[:] }
func (c *scgiConn) unsafeMake(size int) []byte { return c.region.Make(size) }

// scgiRequest
type scgiRequest struct { // outgoing. needs building
	// Assocs
	conn     *scgiConn
	response *scgiResponse
}

func (r *scgiRequest) onUse() {
	// TODO
}
func (r *scgiRequest) onEnd() {
	// TODO
}

// scgiResponse must implements the backendResponse interface.
type scgiResponse struct { // incoming. needs parsing
	// Assocs
	conn *scgiConn
}

func (r *scgiResponse) onUse() {
	// TODO
}
func (r *scgiResponse) onEnd() {
	// TODO
}

//////////////////////////////////////// SCGI protocol elements ////////////////////////////////////////
