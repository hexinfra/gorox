// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package pack

import (
	"github.com/hexinfra/gorox/hemi/contrib/handlets/sitex"

	. "github.com/hexinfra/gorox/hemi"
)

type Pack struct {
	sitex.Pack_
}

func (p *Pack) GET_example(req Request, resp Response) {
	resp.Send("example")
}
