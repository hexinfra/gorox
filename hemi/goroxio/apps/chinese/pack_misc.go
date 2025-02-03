// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package chinese

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (p *Pack) GET_example(req ServerRequest, resp ServerResponse) { // GET /example
	resp.Send("get example")
}
func (p *Pack) POST_foo_bar(req ServerRequest, resp ServerResponse) { // POST /foo/bar
	resp.Echo("foo")
	resp.Echo("bar")
}
