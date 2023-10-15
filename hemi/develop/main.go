// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Develop server (leader & worker) and its control client.

package main

import (
	"os"

	"github.com/hexinfra/gorox/hemi/develop/test"
	"github.com/hexinfra/gorox/hemi/procman"

	_ "github.com/hexinfra/gorox/hemi/develop/apps"
	_ "github.com/hexinfra/gorox/hemi/develop/exts"
	_ "github.com/hexinfra/gorox/hemi/develop/jobs"
	_ "github.com/hexinfra/gorox/hemi/develop/srvs"
	_ "github.com/hexinfra/gorox/hemi/develop/svcs"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "test" {
		test.Main()
	} else {
		procman.Main(&procman.Setting{
			Title:      "Develop",
			Program:    "develop",
			DebugLevel: 2,
			CmdUIAddr:  "127.0.0.1:9523",
			WebUIAddr:  "127.0.0.1:9524",
		})
	}
}
