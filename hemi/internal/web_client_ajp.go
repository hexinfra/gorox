// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// AJP client implementation.

// See: https://tomcat.apache.org/connectors-doc/ajp/ajpv13a.html

// I'm not sure whether AJP supports HTTP unsized content.
// If it doesn't, we have to buffer the request content.

// It seems AJP does support unsized request content, see below:

// Get Body Chunk

// The container asks for more data from the request (If the body was too large to fit in the first packet sent over or when the request is chuncked). The server will send a body packet back with an amount of data which is the minimum of the request_length, the maximum send body size (8186 (8 Kbytes - 6)), and the number of bytes actually left to send from the request body.
// If there is no more data in the body (i.e. the servlet container is trying to read past the end of the body), the server will send back an "empty" packet, which is a body packet with a payload length of 0. (0x12,0x34,0x00,0x00)

package internal

import (
	"sync"
)

// poolAJPExchan
var poolAJPExchan sync.Pool

func getAJPExchan(proxy *ajpProxy, conn *TConn) *ajpExchan {
	var exchan *ajpExchan
	if x := poolAJPExchan.Get(); x == nil {
		exchan = new(ajpExchan)
		req, resp := &exchan.request, &exchan.response
		req.exchan = exchan
		req.response = resp
		resp.exchan = exchan
	} else {
		exchan = x.(*ajpExchan)
	}
	exchan.onUse(proxy, conn)
	return exchan
}
func putAJPExchan(exchan *ajpExchan) {
	exchan.onEnd()
	poolAJPExchan.Put(exchan)
}

// ajpExchan
type ajpExchan struct {
	// Assocs
	request  ajpRequest  // the ajp request
	response ajpResponse // the ajp response
}

func (x *ajpExchan) onUse(proxy *ajpProxy, conn *TConn) {
}
func (x *ajpExchan) onEnd() {
}

// ajpRequest
type ajpRequest struct { // outgoing. needs building
	// Assocs
	exchan   *ajpExchan
	response *ajpResponse
}

// ajpResponse
type ajpResponse struct { // incoming. needs parsing
	// Assocs
	exchan *ajpExchan
}

//////////////////////////////////////// AJP protocol elements ////////////////////////////////////////
