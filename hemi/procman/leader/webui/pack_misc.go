// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package webui

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (p *Pack) GET_example(req Request, resp Response) { // GET /example
	resp.Send("get example")
}
func (p *Pack) POST_foo_bar(req Request, resp Response) { // POST /foo/bar
	resp.Echo("foo")
	resp.Echo("bar")
}
