// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UWSGI client implementation.

// UWSGI is mainly for Python applications. See: https://uwsgi-docs.readthedocs.io/en/latest/Protocol.html
// UWSGI 1.9.13 seems to have unsized content support: https://uwsgi-docs.readthedocs.io/en/latest/Chunked.html

package internal

// uwsgiExchan
type uwsgiExchan struct {
	// Assocs
	request  uwsgiRequest  // the uwsgi request
	response uwsgiResponse // the uwsgi response
}

// uwsgiRequest
type uwsgiRequest struct { // outgoing. needs building
	// TODO
}

// uwsgiResponse
type uwsgiResponse struct { // incoming. needs parsing
	// TODO
}

//////////////////////////////////////// UWSGI protocol elements ////////////////////////////////////////
