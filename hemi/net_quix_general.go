// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIX (QUIC over UDP/UDS) types. See RFC 8999, RFC 9000, RFC 9001, and RFC 9002.

package hemi

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/library/gotcp2"
)

// quixHolder is the interface for _quixHolder_.
type quixHolder interface {
	MaxCumulativeStreamsPerConn() int32
	MaxConcurrentStreamsPerConn() int32
}

// _quixHolder_ is a mixin.
type _quixHolder_ struct { // for quixNode, QUIXRouter, and QUIXGate
	// States
	maxCumulativeStreamsPerConn int32 // max cumulative streams of one conn. 0 means infinite
	maxConcurrentStreamsPerConn int32 // max concurrent streams of one conn
}

func (h *_quixHolder_) onConfigure(comp Component) {
	// .maxCumulativeStreamsPerConn
	comp.ConfigureInt32("maxCumulativeStreamsPerConn", &h.maxCumulativeStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxCumulativeStreamsPerConn has an invalid value")
	}, 1000)

	// .maxConcurrentStreamsPerConn
	comp.ConfigureInt32("maxConcurrentStreamsPerConn", &h.maxConcurrentStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxConcurrentStreamsPerConn has an invalid value")
	}, 1000)
}
func (h *_quixHolder_) onPrepare(comp Component) {
}

func (h *_quixHolder_) MaxCumulativeStreamsPerConn() int32 { return h.maxCumulativeStreamsPerConn }
func (h *_quixHolder_) MaxConcurrentStreamsPerConn() int32 { return h.maxConcurrentStreamsPerConn }

// quixConn collects shared methods between *QUIXConn and *QConn.
type quixConn interface {
}

// quixConn_ is a parent.
type quixConn_ struct { // for QUIXConn and QConn
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id                   int64  // the conn id
	stage                *Stage // current stage, for convenience
	quicConn             *gotcp2.Conn
	udsMode              bool  // for convenience
	tlsMode              bool  // for convenience
	maxCumulativeStreams int32 // how many streams are allowed on this connection?
	maxConcurrentStreams int32 // how many concurrent streams are allowed on this connection?
	// Conn states (zeros)
	counter           atomic.Int64 // can be used to generate a random number
	lastRead          time.Time    // deadline of last read operation
	lastWrite         time.Time    // deadline of last write operation
	broken            atomic.Bool  // is connection broken?
	cumulativeStreams atomic.Int32 // how many streams have been used?
	concurrentStreams atomic.Int32 // how many concurrent streams?
}

func (c *quixConn_) onGet(id int64, stage *Stage, quicConn *gotcp2.Conn, udsMode bool, tlsMode bool, maxCumulativeStreams int32, maxConcurrentStreams int32) {
	c.id = id
	c.stage = stage
	c.quicConn = quicConn
	c.udsMode = udsMode
	c.tlsMode = tlsMode
	c.maxCumulativeStreams = maxCumulativeStreams
	c.maxConcurrentStreams = maxConcurrentStreams
}
func (c *quixConn_) onPut() {
	c.stage = nil
	c.quicConn = nil
	c.counter.Store(0)
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
	c.broken.Store(false)
	c.cumulativeStreams.Store(0)
	c.concurrentStreams.Store(0)
}

func (c *quixConn_) UDSMode() bool { return c.udsMode }
func (c *quixConn_) TLSMode() bool { return c.tlsMode }

func (c *quixConn_) MakeTempName(dst []byte, unixTime int64) int {
	return makeTempName(dst, c.stage.ID(), unixTime, c.id, c.counter.Add(1))
}

func (c *quixConn_) markBroken()    { c.broken.Store(true) }
func (c *quixConn_) isBroken() bool { return c.broken.Load() }

// quixStream collects shared methods between *QUIXStream and *QStream.
type quixStream interface {
}

// quixStream_ is a parent.
type quixStream_ struct { // for QUIXStream and QStream
	// Stream states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Stream states (controlled)
	// Stream states (non-zeros)
	quicStream *gotcp2.Stream
	// Stream states (zeros)
}

func (s *quixStream_) onUse(quicStream *gotcp2.Stream) {
	s.quicStream = quicStream
}
func (s *quixStream_) onEnd() {
	s.quicStream = nil
}
