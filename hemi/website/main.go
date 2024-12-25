// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Website server (leader & worker) and its control client.

package main

import (
	"github.com/hexinfra/gorox/hemi/procmgr"

	_ "github.com/hexinfra/gorox/hemi/website/apps"
)

func main() {
	procmgr.Main(&procmgr.Opts{
		ProgramName:  "website",
		ProgramTitle: "Website",
		DebugLevel:   1,
		CmdUIAddr:    "127.0.0.1:9525",
		WebUIAddr:    "127.0.0.1:9526",
	})
}
