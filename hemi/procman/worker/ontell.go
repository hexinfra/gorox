// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Tell callbacks.

package worker

import (
	"os"
	"runtime"

	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

var onTells = map[uint8]func(req *msgx.Message){
	common.ComdQuit: func(req *msgx.Message) {
		curStage.Quit() // blocking
		os.Exit(0)
	},
	common.ComdCPU: func(req *msgx.Message) {
		curStage.ProfCPU()
	},
	common.ComdHeap: func(req *msgx.Message) {
		curStage.ProfHeap()
	},
	common.ComdThread: func(req *msgx.Message) {
		curStage.ProfThread()
	},
	common.ComdGoroutine: func(req *msgx.Message) {
		curStage.ProfGoroutine()
	},
	common.ComdBlock: func(req *msgx.Message) {
		curStage.ProfBlock()
	},
	common.ComdGC: func(req *msgx.Message) {
		runtime.GC()
	},
}
