// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package tests

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (h *testHandlet) GET_useragent(req Request, resp Response) {
	resp.Send(req.UserAgent())
}
func (h *testHandlet) GET_authority(req Request, resp Response) {
	resp.SendBytes(req.UnsafeAuthority())
}
