// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI responses.

package fcgi

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

// fcgiResponse
type fcgiResponse struct {
	// Assocs
	conn   PConn
	stream *fcgiStream
	// States
	stockInput [2048]byte
	stockArray [1024]byte
	input      []byte
	array      []byte
}

func (r *fcgiResponse) onUse(conn PConn) {
	r.conn = conn
	r.input = r.stockInput[:]
	r.array = r.stockArray[:]
}
func (r *fcgiResponse) onEnd() {
	r.conn = nil
}

func (r *fcgiResponse) recvHead() {
}
func (r *fcgiResponse) addHeader() bool {
	return false
}
func (r *fcgiResponse) checkHead() bool {
	return false
}

func (r *fcgiResponse) walkHeaders() {
}

func (r *fcgiResponse) setMaxRecvSeconds(seconds int64) {
}

func (r *fcgiResponse) readContent() (from int, edge int, err error) {
	return
}

func (r *fcgiResponse) recvContent() any { // to []byte (for small content) or TempFile (for large content)
	return nil
}
func (r *fcgiResponse) holdContent() any {
	return nil
}

func (r *fcgiResponse) newTempFile() {
}

func (r *fcgiResponse) prepareRead() error {
	return nil
}
