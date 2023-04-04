// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package pack

import (
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/handlets/sitex"
)

type Pack struct {
	sitex.Pack_
}

func (p *Pack) OPTIONS_index(req Request, resp Response) {
	if req.IsAsteriskOptions() {
		resp.Send("this is OPTIONS *")
	} else {
		resp.Send("this is OPTIONS / or OPTIONS /index")
	}
}
func (p *Pack) GET_example(req Request, resp Response) { // GET /example
	resp.Send("get example")
}
func (p *Pack) POST_foo_bar(req Request, resp Response) { // POST /foo/bar
	resp.Echo("foo")
	resp.Echo("bar")
}
