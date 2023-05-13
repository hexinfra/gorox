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

var onTells = map[uint8]func(stage *hemi.Stage, req *msgx.Message){
	common.ComdQuit: func(stage *hemi.Stage, req *msgx.Message) {
		stage.Quit() // blocking
		os.Exit(0)
	},
	common.ComdReload: func(stage *hemi.Stage, req *msgx.Message) {
		if newStage, err := hemi.ApplyFile(configBase, configFile); err == nil {
			id := stage.ID() + 1
			newStage.Start(id)
			currentStage = newStage
			stage.Quit()
		}
	},
	common.ComdCPU: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfCPU()
	},
	common.ComdHeap: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfHeap()
	},
	common.ComdThread: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfThread()
	},
	common.ComdGoroutine: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfGoroutine()
	},
	common.ComdBlock: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfBlock()
	},
	common.ComdGC: func(stage *hemi.Stage, req *msgx.Message) {
		runtime.GC()
	},
}
