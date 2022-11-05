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

func (c *Controller) OPTIONS_index(req Request, resp Response) {
	if req.IsAsteriskOptions() {
		resp.Send("this is OPTIONS *")
	} else {
		resp.Send("this is OPTIONS / or OPTIONS /index")
	}
}
func (c *Controller) GET_example(req Request, resp Response) { // GET /example
	resp.Send("get example")
}
func (c *Controller) POST_foo_bar(req Request, resp Response) { // POST /foo/bar
	resp.Push("foo")
	resp.Push("bar")
}
