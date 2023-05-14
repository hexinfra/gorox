// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Call callbacks.

package worker

import (
	"os"
	"runtime"
	"strconv"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

var onCalls = map[uint8]func(stage *hemi.Stage, req *msgx.Message, resp *msgx.Message){
	common.ComdPids: func(stage *hemi.Stage, req *msgx.Message, resp *msgx.Message) {
		resp.Set("worker", strconv.Itoa(os.Getpid()))
	},
	common.ComdWorker: func(stage *hemi.Stage, req *msgx.Message, resp *msgx.Message) {
		resp.Set("goroutines", strconv.Itoa(runtime.NumGoroutine())) // TODO: other infos
	},
}