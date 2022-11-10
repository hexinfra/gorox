// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package test

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (h *testHandler) GET_(req Request, resp Response) { // GET empty through absolute-form, or GET /
	if req.IsAbsoluteForm() {
		resp.Send("absolute-form GET /")
	} else {
		resp.Send("origin-form GET /")
	}
}
func (h *testHandler) OPTIONS_(req Request, resp Response) { // OPTIONS * or OPTIONS /
	if req.IsAsteriskOptions() {
		if req.IsAbsoluteForm() {
			resp.Send("absolute-form OPTIONS *")
		} else {
			resp.Send("asterisk-form OPTIONS *")
		}
	} else {
		if req.IsAbsoluteForm() {
			resp.Send("absolute-form OPTIONS /")
		} else {
			resp.Send("origin-form OPTIONS /")
		}
	}
}
