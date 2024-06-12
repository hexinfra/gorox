// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// uwsgi proxy implementation.

// uwsgi is mainly for Python applications. See: https://uwsgi-docs.readthedocs.io/en/latest/Protocol.html
// uwsgi 1.9.13 seems to have vague content support: https://uwsgi-docs.readthedocs.io/en/latest/Chunked.html

package hemi

import (
	"errors"
	"net"
	"sync"
	"time"
)

func init() {
	RegisterHandlet("uwsgiProxy", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(uwsgiProxy)
		h.onCreate(name, stage, webapp)
		return h
	})
	RegisterBackend("uwsgiBackend", func(name string, stage *Stage) Backend {
		b := new(uwsgiBackend)
		b.onCreate(name, stage)
		return b
	})
}

// uwsgiProxy handlet passes web requests to uWSGI backends and caches responses.
type uwsgiProxy struct {
	// Parent
	Handlet_
	// Assocs
	stage   *Stage        // current stage
	webapp  *Webapp       // the webapp to which the proxy belongs
	backend *uwsgiBackend // the backend to pass to
	cacher  Cacher        // the cacher which is used by this proxy
	// States
	WebExchanProxyConfig // embeded
}

func (h *uwsgiProxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *uwsgiProxy) OnShutdown() {
	h.webapp.DecSub()
}

func (h *uwsgiProxy) OnConfigure() {
	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if uwsgiBackend, ok := backend.(*uwsgiBackend); ok {
				h.backend = uwsgiBackend
			} else {
				UseExitf("incorrect backend '%s' for uwsgiProxy, must be uwsgiBackend\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for uwsgiProxy")
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
func (h *uwsgiProxy) OnPrepare() {
}

func (h *uwsgiProxy) IsProxy() bool { return true }
func (h *uwsgiProxy) IsCache() bool { return h.cacher != nil }

func (h *uwsgiProxy) Handle(req Request, resp Response) (handled bool) {
	// TODO: implementation
	resp.Send("uwsgi")
	return true
}

// uwsgiBackend
type uwsgiBackend struct {
	// Parent
	Backend_[*uwsgiNode]
	// Mixins
	_contentSaver_ // so responses can save their large contents in local file system.
	// States
	sendTimeout         time.Duration // timeout to send the whole request
	recvTimeout         time.Duration // timeout to recv the whole response content
	maxContentSize      int64         // max response content size allowed
}

func (b *uwsgiBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *uwsgiBackend) OnConfigure() {
	b.Backend_.OnConfigure()
	b._contentSaver_.onConfigure(b, TmpDir()+"/web/uwsgi/"+b.name)

	// sendTimeout
	b.ConfigureDuration("sendTimeout", &b.sendTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, 60*time.Second)

	// recvTimeout
	b.ConfigureDuration("recvTimeout", &b.recvTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, 60*time.Second)

	// maxContentSize
	b.ConfigureInt64("maxContentSize", &b.maxContentSize, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxContentSize has an invalid value")
	}, _1T)

	// sub components
	b.ConfigureNodes()
}
func (b *uwsgiBackend) OnPrepare() {
	b.Backend_.OnPrepare()
	b._contentSaver_.onPrepare(b, 0755)

	// sub components
	b.PrepareNodes()
}

func (b *uwsgiBackend) CreateNode(name string) Node {
	node := new(uwsgiNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

// uwsgiNode
type uwsgiNode struct {
	// Parent
	Node_
	// Assocs
	// States
}

func (n *uwsgiNode) onCreate(name string, backend *uwsgiBackend) {
	n.Node_.OnCreate(name, backend)
}

func (n *uwsgiNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *uwsgiNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *uwsgiNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	/*
	if size := n.closeFree(); size > 0 {
		n.SubsAddn(-size)
	}
	n.WaitSubs() // conns. TODO: max timeout?
	*/
	if DebugLevel() >= 2 {
		Printf("uwsgiNode=%s done\n", n.name)
	}
	n.backend.DecSub()
}

func (n *uwsgiNode) dial() (*uwsgiConn, error) {
	if DebugLevel() >= 2 {
		Printf("uwsgiNode=%s dial %s\n", n.name, n.address)
	}
	var (
		fConn *uwsgiConn
		err   error
	)
	if n.IsUDS() {
		fConn, err = n._dialUDS()
	} else {
		fConn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSub()
	return fConn, err
}
func (n *uwsgiNode) _dialUDS() (*uwsgiConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("uwsgiNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getUWSGIConn(connID, n, netConn, rawConn), nil
}
func (n *uwsgiNode) _dialTCP() (*uwsgiConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("uwsgiNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getUWSGIConn(connID, n, netConn, rawConn), nil
}

// poolUWSGIConn
var poolUWSGIConn sync.Pool

func getUWSGIConn(proxy *uwsgiProxy, tConn *TConn) *uwsgiConn {
	var conn *uwsgiConn
	if x := poolUWSGIConn.Get(); x == nil {
		conn = new(uwsgiConn)
		req, resp := &conn.request, &conn.response
		req.conn = conn
		req.response = resp
		resp.conn = conn
	} else {
		conn = x.(*uwsgiConn)
	}
	conn.onUse(proxy, tConn)
	return conn
}
func putUWSGIConn(conn *uwsgiConn) {
	conn.onEnd()
	poolUWSGIConn.Put(conn)
}

// uwsgiConn
type uwsgiConn struct {
	// Parent
	BackendConn_
	// Assocs
	request  uwsgiRequest  // the uwsgi request
	response uwsgiResponse // the uwsgi response
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Conn states (controlled)
	// Conn states (non-zeros)
	region Region      // a region-based memory pool
	proxy  *uwsgiProxy // associated proxy
	conn   *TConn      // associated conn
	// Conn states (zeros)
}

func (x *uwsgiConn) onUse(proxy *uwsgiProxy, conn *TConn) {
	x.region.Init()
	x.proxy = proxy
	x.conn = conn
	x.region.Init()
	x.request.onUse()
	x.response.onUse()
}
func (x *uwsgiConn) onEnd() {
	x.request.onEnd()
	x.response.onEnd()
	x.conn = nil
	x.proxy = nil
	x.region.Free()
}

func (x *uwsgiConn) buffer256() []byte          { return x.stockBuffer[:] }
func (x *uwsgiConn) unsafeMake(size int) []byte { return x.region.Make(size) }

// uwsgiRequest
type uwsgiRequest struct { // outgoing. needs building
	// Assocs
	conn   *uwsgiConn
	response *uwsgiResponse
}

func (r *uwsgiRequest) onUse() {
	// TODO
}
func (r *uwsgiRequest) onEnd() {
	// TODO
}

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

//////////////////////////////////////// uwsgi protocol elements ////////////////////////////////////////
