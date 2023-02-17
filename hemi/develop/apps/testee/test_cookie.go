// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package testee

import (
	. "github.com/hexinfra/gorox/hemi"
	"time"
)

func (h *testHandlet) GET_cookie_set(req Request, resp Response) {
	cookie1 := new(SetCookie)
	cookie1.Set("hello", "wo r,ld")
	cookie1.SetMaxAge(99)
	cookie1.SetExpires(time.Now().Add(time.Minute))
	cookie1.SetPath("/")
	resp.SetCookie(cookie1)

	cookie2 := new(SetCookie)
	cookie2.Set("world", "hello")
	cookie2.SetPath("/")
	resp.SetCookie(cookie2)

	resp.SendBytes(nil)
}
func (h *testHandlet) GET_cookies(req Request, resp Response) {
	resp.Push(req.C("hello"))
	resp.Push(req.C("world"))
}
