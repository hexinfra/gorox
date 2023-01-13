// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI requests.

package fcgi

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

// fcgiRequest
type fcgiRequest struct {
	// Assocs
	conn     PConn
	stream   *fcgiStream
	response *fcgiResponse
	// States
}

func (r *fcgiRequest) onUse(conn PConn) {
	r.conn = conn
}
func (r *fcgiRequest) onEnd() {
	r.conn = nil
}

func (r *fcgiRequest) withHead(req Request) bool {
	return false
}

func (r *fcgiRequest) sync(req Request) error {
	return nil
}
func (r *fcgiRequest) pass(content any) error { // nil, []byte, *os.File
	return nil
}
