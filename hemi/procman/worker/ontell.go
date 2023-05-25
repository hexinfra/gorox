// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Tell callbacks.

package worker

import (
	"os"
	"runtime"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

var onTells = map[uint8]func(req *msgx.Message){
	common.ComdQuit: func(req *msgx.Message) {
		currentStage.Quit() // blocking
		os.Exit(0)
	},
	common.ComdReload: func(req *msgx.Message) {
		if newStage, err := hemi.ApplyFile(configBase, configFile); err == nil {
			oldStage := currentStage
			newStage.Start(oldStage.ID() + 1)
			currentStage = newStage
			oldStage.Quit()
		}
	},
	common.ComdCPU: func(req *msgx.Message) {
		currentStage.ProfCPU()
	},
	common.ComdHeap: func(req *msgx.Message) {
		currentStage.ProfHeap()
	},
	common.ComdThread: func(req *msgx.Message) {
		currentStage.ProfThread()
	},
	common.ComdGoroutine: func(req *msgx.Message) {
		currentStage.ProfGoroutine()
	},
	common.ComdBlock: func(req *msgx.Message) {
		currentStage.ProfBlock()
	},
	common.ComdGC: func(req *msgx.Message) {
		runtime.GC()
	},
}
