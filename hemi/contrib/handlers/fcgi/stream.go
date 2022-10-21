// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI streams.

package fcgi

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"sync"
	"time"
)

// poolFCGIStream
var poolFCGIStream sync.Pool

func getFCGIStream(conn PConn) *fcgiStream {
	var stream *fcgiStream
	if x := poolFCGIStream.Get(); x == nil {
		stream = new(fcgiStream)
	} else {
		stream = x.(*fcgiStream)
	}
	stream.onUse(conn)
	return stream
}
func putFCGIStream(stream *fcgiStream) {
	stream.onEnd()
	poolFCGIStream.Put(stream)
}

// fcgiStream
type fcgiStream struct {
	// Assocs
	request  fcgiRequest  // the fcgi request
	response fcgiResponse // the fcgi response
	// Stream states (buffers)
	stockStack [64]byte
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn PConn
}

func (s *fcgiStream) onUse(conn PConn) {
	s.conn = conn
	s.request.onUse(conn)
	s.response.onUse(conn)
}
func (s *fcgiStream) onEnd() {
	s.request.onEnd()
	s.response.onEnd()
	s.conn = nil
}

func (s *fcgiStream) smallStack() []byte {
	return s.stockStack[:]
}

func (s *fcgiStream) setWriteDeadline(deadline time.Time) error {
	return nil
}
func (s *fcgiStream) setReadDeadline(deadline time.Time) error {
	return nil
}

func (s *fcgiStream) write(p []byte) (int, error) {
	return s.conn.Write(p)
}
func (s *fcgiStream) read(p []byte) (int, error) {
	return s.conn.Read(p)
}
