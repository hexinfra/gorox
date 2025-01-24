// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// UDPX (UDP/UDS) types. See RFC 768 and RFC 8085.

package hemi

import (
	"net"
	"sync/atomic"
	"syscall"
	"time"
)

// udpxHolder is the interface for _udpxHolder_.
type udpxHolder interface {
}

// _udpxHolder_ is a mixin for UDPXRouter, UDPXGate, and udpxNode.
type _udpxHolder_ struct {
	// States
	// UDP_CORK, UDP_GSO, ...
}

func (h *_udpxHolder_) onConfigure(comp Component) {
}
func (h *_udpxHolder_) onPrepare(comp Component) {
}

// udpxConn collects shared methods between *UDPXConn and *UConn.
type udpxConn interface {
}

// udpxConn_ is the parent for UDPXConn and UConn.
type udpxConn_ struct {
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64  // the conn id
	stage   *Stage // current stage, for convenience
	udsMode bool   // for convenience
	pktConn net.PacketConn
	rawConn syscall.RawConn // for syscall
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastRead  time.Time    // deadline of last read operation
	lastWrite time.Time    // deadline of last write operation
	broken    atomic.Bool
}

func (c *udpxConn_) onGet(id int64, stage *Stage, pktConn net.PacketConn, rawConn syscall.RawConn, udsMode bool) {
	c.id = id
	c.stage = stage
	c.pktConn = pktConn
	c.rawConn = rawConn
	c.udsMode = udsMode
}
func (c *udpxConn_) onPut() {
	c.stage = nil
	c.pktConn = nil
	c.rawConn = nil
	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
	c.broken.Store(false)
}

func (c *udpxConn_) UDSMode() bool { return c.udsMode }

func (c *udpxConn_) MakeTempName(dst []byte, unixTime int64) int {
	return makeTempName(dst, c.stage.ID(), unixTime, c.id, c.counter.Add(1))
}

func (c *udpxConn_) markBroken()    { c.broken.Store(true) }
func (c *udpxConn_) isBroken() bool { return c.broken.Load() }

func (c *udpxConn_) WriteTo(src []byte, addr net.Addr) (n int, err error) {
	return c.pktConn.WriteTo(src, addr)
}
func (c *udpxConn_) ReadFrom(dst []byte) (n int, addr net.Addr, err error) {
	return c.pktConn.ReadFrom(dst)
}
