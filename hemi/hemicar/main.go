// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HemiCar server (leader & worker) and its control client.

package main

import (
	"github.com/hexinfra/gorox/hemi/procman"

	_ "github.com/hexinfra/gorox/hemi/hemicar/apps"
	_ "github.com/hexinfra/gorox/hemi/hemicar/exts"
	_ "github.com/hexinfra/gorox/hemi/hemicar/svcs"
)

func main() {
	procman.Main(&procman.Opts{
		ProgramName:  "hemicar",
		ProgramTitle: "HemiCar",
		DebugLevel:   2,
		CmdUIAddr:    "127.0.0.1:9523",
		WebUIAddr:    "127.0.0.1:9524",
	})
}
