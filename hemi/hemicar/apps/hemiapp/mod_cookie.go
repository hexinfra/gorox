// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package hemiapp

import (
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func (h *hemiappHandlet) GET_cookie_set(req ServerRequest, resp ServerResponse) {
	cookie1 := new(Cookie)
	cookie1.Set("hello", "wo r,ld")
	cookie1.SetMaxAge(99)
	cookie1.SetExpires(time.Now().Add(time.Minute))
	cookie1.SetPath("/")
	resp.AddCookie(cookie1)

	cookie2 := new(Cookie)
	cookie2.Set("world", "hello")
	cookie2.SetPath("/")
	resp.AddCookie(cookie2)

	resp.SendBytes(nil)
}
func (h *hemiappHandlet) GET_cookies(req ServerRequest, resp ServerResponse) {
	resp.Echo(req.C("hello"))
	resp.Echo(req.C("world"))
}
