// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS client implementation.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	registerFixture(signUDPSOutgate)
	RegisterBackend("udpsBackend", func(name string, stage *Stage) Backend {
		b := new(UDPSBackend)
		b.onCreate(name, stage)
		return b
	})
}

// udpsClient is the interface for UDPSOutgate and UDPSBackend.
type udpsClient interface {
	client
}

const signUDPSOutgate = "udpsOutgate"

func createUDPSOutgate(stage *Stage) *UDPSOutgate {
	udps := new(UDPSOutgate)
	udps.onCreate(stage)
	udps.setShell(udps)
	return udps
}

// UDPSOutgate component.
type UDPSOutgate struct {
	// Mixins
	outgate_
	// States
}

func (f *UDPSOutgate) onCreate(stage *Stage) {
	f.outgate_.onCreate(signUDPSOutgate, stage)
}

func (f *UDPSOutgate) OnConfigure() {
	f.outgate_.onConfigure()
}
func (f *UDPSOutgate) OnPrepare() {
	f.outgate_.onConfigure()
}

func (f *UDPSOutgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Println("udpsOutgate done")
	}
	f.stage.SubDone()
}

func (f *UDPSOutgate) Dial(address string, tlsMode bool) (*ULink, error) {
	// TODO
	return nil, nil
}
func (f *UDPSOutgate) FetchLink(address string, tlsMode bool) (*ULink, error) {
	// TODO
	return nil, nil
}
func (f *UDPSOutgate) StoreLink(link *ULink) {
	// TODO
}

// UDPSBackend component.
type UDPSBackend struct {
	// Mixins
	Backend_[*udpsNode]
	loadBalancer_
	// States
	health any // TODO
}

func (b *UDPSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.onCreate(name, stage, b)
	b.loadBalancer_.init()
}

func (b *UDPSBackend) OnConfigure() {
	b.Backend_.onConfigure()
	b.loadBalancer_.onConfigure(b)
}
func (b *UDPSBackend) OnPrepare() {
	b.Backend_.onPrepare()
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *UDPSBackend) createNode(id int32) *udpsNode {
	node := new(udpsNode)
	node.init(id, b)
	return node
}

func (b *UDPSBackend) Link() (*ULink, error) {
	node := b.nodes[b.getNext()]
	return node.link()
}
func (b *UDPSBackend) FetchLink() (*ULink, error) {
	node := b.nodes[b.getNext()]
	return node.fetchLink()
}
func (b *UDPSBackend) StoreLink(uLink *ULink) {
	uLink.node.storeLink(uLink)
}

// udpsNode is a node in UDPSBackend.
type udpsNode struct {
	// Mixins
	Node_
	// Assocs
	backend *UDPSBackend
	// States
}

func (n *udpsNode) init(id int32, backend *UDPSBackend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *udpsNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all links
	if IsDebug(2) {
		Printf("udpsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *udpsNode) link() (*ULink, error) {
	// TODO
	return nil, nil
}
func (n *udpsNode) fetchLink() (*ULink, error) {
	link := n.pullConn()
	if link != nil {
		uLink := link.(*ULink)
		if uLink.isAlive() {
			return uLink, nil
		}
		uLink.closeConn()
		putULink(uLink)
	}
	return n.link()
}
func (n *udpsNode) storeLink(uLink *ULink) {
	if uLink.isBroken() || n.isDown() || !uLink.isAlive() {
		uLink.closeConn()
		putULink(uLink)
	} else {
		n.pushConn(uLink)
	}
}

// poolULink
var poolULink sync.Pool

func getULink(id int64, client udpsClient, node *udpsNode, udpConn *net.UDPConn, rawConn syscall.RawConn) *ULink {
	var link *ULink
	if x := poolULink.Get(); x == nil {
		link = new(ULink)
	} else {
		link = x.(*ULink)
	}
	link.onGet(id, client, node, udpConn, rawConn)
	return link
}
func putULink(link *ULink) {
	link.onPut()
	poolULink.Put(link)
}

// ULink is a client-side link to udpsNode.
type ULink struct { // only exported to hemi
	// Mixins
	conn_
	// Link states (non-zeros)
	node    *udpsNode       // associated node if client is UDPSBackend
	udpConn *net.UDPConn    // udp conn
	rawConn syscall.RawConn // for syscall
	// Link states (zeros)
	broken atomic.Bool // is link broken?
}

func (l *ULink) onGet(id int64, client udpsClient, node *udpsNode, udpConn *net.UDPConn, rawConn syscall.RawConn) {
	l.conn_.onGet(id, client)
	l.node = node
	l.udpConn = udpConn
	l.rawConn = rawConn
}
func (l *ULink) onPut() {
	l.conn_.onPut()
	l.node = nil
	l.udpConn = nil
	l.rawConn = nil
	l.broken.Store(false)
}

func (l *ULink) getClient() udpsClient { return l.client.(udpsClient) }

func (l *ULink) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(l.lastWrite) >= time.Second {
		if err := l.udpConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		l.lastWrite = deadline
	}
	return nil
}
func (l *ULink) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(l.lastRead) >= time.Second {
		if err := l.udpConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		l.lastRead = deadline
	}
	return nil
}

func (l *ULink) Write(p []byte) (n int, err error) { return l.udpConn.Write(p) }
func (l *ULink) Read(p []byte) (n int, err error)  { return l.udpConn.Read(p) }

func (l *ULink) isBroken() bool { return l.broken.Load() }
func (l *ULink) markBroken()    { l.broken.Store(true) }

func (l *ULink) Close() error { // only used by clients of dial
	udpConn := l.udpConn
	putULink(l)
	return udpConn.Close()
}

func (l *ULink) closeConn() { l.udpConn.Close() } // used by codes other than dial
