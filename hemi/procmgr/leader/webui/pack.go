// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package webui

import (
	"github.com/hexinfra/gorox/hemi/options/handlets/sitex"

	. "github.com/hexinfra/gorox/hemi"
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
