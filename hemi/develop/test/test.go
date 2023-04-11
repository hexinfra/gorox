// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Tests.

package test

import (
	"github.com/hexinfra/gorox/hemi/develop/test/apps/develop"
	"github.com/hexinfra/gorox/hemi/develop/test/apps/diogin"
	"github.com/hexinfra/gorox/hemi/develop/test/apps/fengve"
	"os"
)

func Main() {
	tests := ""
	if len(os.Args) >= 3 {
		tests = os.Args[2]
	}
	switch tests {
	case "fengve":
		fengve.Main()
	case "diogin":
		diogin.Main()
	default:
		develop.Main()
	}
}
