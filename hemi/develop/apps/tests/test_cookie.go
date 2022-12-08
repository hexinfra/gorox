// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package tests

import (
	. "github.com/hexinfra/gorox/hemi"
	"time"
)

func (h *testHandler) GET_cookie_set(req Request, resp Response) {
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
func (h *testHandler) GET_cookies(req Request, resp Response) {
	resp.Push(req.C("hello"))
	resp.Push(req.C("world"))
}
