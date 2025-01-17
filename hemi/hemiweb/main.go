// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HemiWeb server (leader & worker) and its control client.

package main

import (
	"github.com/hexinfra/gorox/hemi/procmgr"

	_ "github.com/hexinfra/gorox/hemi/hemiweb/apps"
	_ "github.com/hexinfra/gorox/hemi/hemiweb/exts"
)

func main() {
	procmgr.Main(&procmgr.Opts{
		ProgramName:  "hemiweb",
		ProgramTitle: "HemiWeb",
		DebugLevel:   1,
		CmdUIAddr:    "127.0.0.1:9525",
		WebUIAddr:    "127.0.0.1:9526",
	})
}
