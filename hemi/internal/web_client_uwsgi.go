// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UWSGI client implementation.

// UWSGI is mainly for Python applications. See: https://uwsgi-docs.readthedocs.io/en/latest/Protocol.html
// UWSGI 1.9.13 seems to have unsized content support: https://uwsgi-docs.readthedocs.io/en/latest/Chunked.html

package internal

import (
	"sync"
)

// poolUWSGIExchan
var poolUWSGIExchan sync.Pool

func getUWSGIExchan(proxy *uwsgiProxy, conn wireConn) *uwsgiExchan {
	var exchan *uwsgiExchan
	if x := poolUWSGIExchan.Get(); x == nil {
		exchan = new(uwsgiExchan)
		req, resp := &exchan.request, &exchan.response
		req.exchan = exchan
		req.response = resp
		resp.exchan = exchan
	} else {
		exchan = x.(*uwsgiExchan)
	}
	exchan.onUse(proxy, conn)
	return exchan
}
func putUWSGIExchan(exchan *uwsgiExchan) {
	exchan.onEnd()
	poolUWSGIExchan.Put(exchan)
}

// uwsgiExchan
type uwsgiExchan struct {
	// Assocs
	request  uwsgiRequest  // the uwsgi request
	response uwsgiResponse // the uwsgi response
}

func (x *uwsgiExchan) onUse(proxy *uwsgiProxy, conn wireConn) {
}
func (x *uwsgiExchan) onEnd() {
}

// uwsgiRequest
type uwsgiRequest struct { // outgoing. needs building
	// Assocs
	exchan   *uwsgiExchan
	response *uwsgiResponse
}

// uwsgiResponse
type uwsgiResponse struct { // incoming. needs parsing
	// Assocs
	exchan *uwsgiExchan
}

//////////////////////////////////////// UWSGI protocol elements ////////////////////////////////////////
