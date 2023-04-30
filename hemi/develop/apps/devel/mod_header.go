// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package devel

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (h *develHandlet) GET_useragent(req Request, resp Response) {
	resp.Send(req.UserAgent())
}
func (h *develHandlet) GET_authority(req Request, resp Response) {
	resp.SendBytes(req.UnsafeAuthority())
}
func (h *develHandlet) GET_single(req Request, resp Response) {
	accept, ok := req.Header("accept")
	if !ok {
		resp.Send("please provide accept header")
		return
	}
	resp.Send(accept)
}
func (h *develHandlet) GET_multi(req Request, resp Response) {
	accepts, ok := req.Headers("accept")
	if !ok {
		resp.Send("please provide accept header")
		return
	}
	for _, accept := range accepts {
		resp.Echo(accept + "<br>")
	}
}
func (h *develHandlet) GET_multi2(req Request, resp Response) {
	uas, ok := req.Headers("sec-ch-ua")
	if !ok {
		resp.Send("please provide sec-ch-ua header")
		return
	}
	for _, ua := range uas {
		resp.Echo(ua + "<br>")
	}
}