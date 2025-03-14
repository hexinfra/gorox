// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPX (TCP/TLS/UDS) types. See RFC 9293.

package hemi

import (
	"net"
	"sync/atomic"
	"syscall"
	"time"
)

// tcpxHolder is the interface for _tcpxHolder_.
type tcpxHolder interface {
}

// _tcpxHolder_ is a mixin.
type _tcpxHolder_ struct { // for tcpxNode, TCPXRouter, and TCPXGate
	// States
	// TCP_CORK, TCP_DEFER_ACCEPT, TCP_FASTOPEN, ...
}

func (h *_tcpxHolder_) onConfigure(comp Component) {
}
func (h *_tcpxHolder_) onPrepare(comp Component) {
}

// tcpxConn collects shared methods between *TCPXConn and *TConn.
type tcpxConn interface {
}

// tcpxConn_ is a parent.
type tcpxConn_ struct { // for TCPXConn and TConn
	// Conn states (stocks)
	stockBuffer [256]byte  // a (fake) buffer to workaround Go's conservative escape analysis
	stockInput  [8192]byte // for c.input
	// Conn states (controlled)
	// Conn states (non-zeros)
	id           int64           // the conn id
	stage        *Stage          // current stage, for convenience
	udsMode      bool            // for convenience
	tlsMode      bool            // for convenience
	readTimeout  time.Duration   // for convenience
	writeTimeout time.Duration   // for convenience
	netConn      net.Conn        // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn      syscall.RawConn // for syscall, only usable when netConn is TCP/UDS
	input        []byte          // input buffer
	region       Region          // a region-based memory pool
	closeSema    atomic.Int32    // controls read/write close
	// Conn states (zeros)
	counter     atomic.Int64 // can be used to generate a random number
	lastRead    time.Time    // deadline of last read operation
	lastWrite   time.Time    // deadline of last write operation
	broken      atomic.Bool  // is connection broken?
	Vector      net.Buffers  // used by Sendv()
	FixedVector [4][]byte    // used by Sendv()
}

func (c *tcpxConn_) onGet(id int64, holder holder, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.stage = holder.Stage()
	c.udsMode = holder.UDSMode()
	c.tlsMode = holder.TLSMode()
	c.readTimeout = holder.ReadTimeout()
	c.writeTimeout = holder.WriteTimeout()
	c.netConn = netConn
	c.rawConn = rawConn
	c.input = c.stockInput[:]
	c.region.Init()
	c.closeSema.Store(2)
}
func (c *tcpxConn_) onPut() {
	c.stage = nil
	c.netConn = nil
	c.rawConn = nil
	if cap(c.input) != cap(c.stockInput) {
		PutNK(c.input)
	}
	c.input = nil
	c.region.Free()

	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
	c.broken.Store(false)
	c.Vector = nil
	c.FixedVector = [4][]byte{}
}

func (c *tcpxConn_) UDSMode() bool { return c.udsMode }
func (c *tcpxConn_) TLSMode() bool { return c.tlsMode }

func (c *tcpxConn_) MakeTempName(dst []byte, unixTime int64) int {
	return makeTempName(dst, c.stage.ID(), unixTime, c.id, c.counter.Add(1))
}

func (c *tcpxConn_) markBroken()    { c.broken.Store(true) }
func (c *tcpxConn_) isBroken() bool { return c.broken.Load() }

func (c *tcpxConn_) SetReadDeadline() error {
	if deadline := time.Now().Add(c.readTimeout); deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *tcpxConn_) SetWriteDeadline() error {
	if deadline := time.Now().Add(c.writeTimeout); deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *tcpxConn_) Recv() (data []byte, err error) {
	n, err := c.netConn.Read(c.input)
	data = c.input[:n]
	return
}
func (c *tcpxConn_) Send(data []byte) (err error) {
	_, err = c.netConn.Write(data)
	return
}
func (c *tcpxConn_) Sendv() (err error) {
	_, err = c.Vector.WriteTo(c.netConn)
	return
}
