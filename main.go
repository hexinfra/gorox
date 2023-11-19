// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gorox server (leader & worker) and its control client.

package main

import (
	"github.com/hexinfra/gorox/hemi/procman"

	_ "github.com/hexinfra/gorox/apps"
	_ "github.com/hexinfra/gorox/exts"
	_ "github.com/hexinfra/gorox/jobs"
	_ "github.com/hexinfra/gorox/srvs"
	_ "github.com/hexinfra/gorox/svcs"
)

func main() {
	procman.Main(&procman.Settings{
		Title:      "Gorox",
		Program:    "gorox",
		DebugLevel: 0,
		CmdUIAddr:  "127.0.0.1:9527",
		WebUIAddr:  "127.0.0.1:9528",
	})
}
