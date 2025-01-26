// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Gorox server (leader & worker) and its control client.

package main

import (
	"github.com/hexinfra/gorox/hemi/procmgr"

	_ "github.com/hexinfra/gorox/apps" // all webapps
	_ "github.com/hexinfra/gorox/exts" // all extensions
	_ "github.com/hexinfra/gorox/svcs" // all services
)

func main() {
	procmgr.Main(&procmgr.Opts{
		ProgramName:  "gorox",
		ProgramTitle: "Gorox",
		DebugLevel:   0,
		CmdUIAddr:    "127.0.0.1:9527",
		WebUIAddr:    "127.0.0.1:9528",
	})
}
