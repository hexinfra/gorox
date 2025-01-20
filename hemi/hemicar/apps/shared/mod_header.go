// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package shared

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (h *sharedHandlet) GET_useragent(req ServerRequest, resp ServerResponse) {
	resp.Send(req.UserAgent())
}
func (h *sharedHandlet) GET_authority(req ServerRequest, resp ServerResponse) {
	resp.SendBytes(req.UnsafeAuthority())
}
func (h *sharedHandlet) GET_single(req ServerRequest, resp ServerResponse) {
	accept, ok := req.Header("accept")
	if !ok {
		resp.Send("please provide accept header")
		return
	}
	resp.Send(accept)
}
func (h *sharedHandlet) GET_multi(req ServerRequest, resp ServerResponse) {
	accepts, ok := req.Headers("accept")
	if !ok {
		resp.Send("please provide accept header")
		return
	}
	for _, accept := range accepts {
		resp.Echo(accept + "<br>")
	}
}
func (h *sharedHandlet) GET_multi2(req ServerRequest, resp ServerResponse) {
	uas, ok := req.Headers("sec-ch-ua")
	if !ok {
		resp.Send("please provide sec-ch-ua header")
		return
	}
	for _, ua := range uas {
		resp.Echo(ua + "<br>")
	}
}
