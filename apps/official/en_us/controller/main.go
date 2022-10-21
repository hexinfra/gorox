// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package controller

import (
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/handlers/sitex"
)

type Controller struct {
	sitex.Controller_
}

func (c *Controller) GET_example(req Request, resp Response) {
	resp.Send("get example")
}
func (c *Controller) POST_example(req Request, resp Response) {
	resp.Send("post example")
}
