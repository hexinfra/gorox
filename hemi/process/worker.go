// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Worker process.

package process

import (
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
)

var (
	configBase   string      // base string of config file
	configFile   string      // config file path
	currentStage *hemi.Stage // current stage
)

// workerMain is main() for worker process.
func workerMain(token string) {
	parts := strings.Split(token, "|") // ip:port|pipeKey
	if len(parts) != 2 {
		crash("bad token")
	}

	// Contact leader process
	cmdPipe, err := net.Dial("tcp", parts[0]) // ip:port
	if err != nil {
		crash("dial leader failed: " + err.Error())
	}

	// Register worker to leader
	if loginResp, ok := msgx.Call(cmdPipe, msgx.NewMessage(0, 0, map[string]string{
		"pipeKey": parts[1],
	}), 16<<20); ok {
		configBase = loginResp.Get("base")
		configFile = loginResp.Get("file")
	} else {
		crash("call leader failed")
	}

	// Register succeeded. Now start the initial stage
	currentStage, err = hemi.ApplyFile(configBase, configFile)
	if err != nil {
		crash(err.Error())
	}
	currentStage.Start(0)

	// Stage started, now waiting for leader's commands.
	for { // each message from leader process
		req, ok := msgx.Recv(cmdPipe, 16<<20)
		if !ok { // leader must be gone
			break
		}
		if hemi.IsDebug(2) {
			hemi.Debugf("worker received req=%v\n", req)
		}
		if req.IsCall() {
			resp := msgx.NewMessage(req.Comd, 0, nil)
			if onCall, ok := onCalls[req.Comd]; ok {
				onCall(currentStage, req, resp)
			} else {
				resp.Flag = 404
			}
			if !msgx.Send(cmdPipe, resp) { // leader must be gone
				break
			}
		} else if onTell, ok := onTells[req.Comd]; ok {
			onTell(currentStage, req)
		} else {
			// Unknown tell command, ignore.
		}
	}

	stop() // the loop is broken. simply stop worker
}

var onCalls = map[uint8]func(stage *hemi.Stage, req *msgx.Message, resp *msgx.Message){ // call commands
	comdPid: func(stage *hemi.Stage, req *msgx.Message, resp *msgx.Message) {
		resp.Set("worker", strconv.Itoa(os.Getpid()))
	},
	comdInfo: func(stage *hemi.Stage, req *msgx.Message, resp *msgx.Message) {
		resp.Set("goroutines", strconv.Itoa(runtime.NumGoroutine())) // TODO: other infos
	},
}

var onTells = map[uint8]func(stage *hemi.Stage, req *msgx.Message){ // tell commands
	comdQuit: func(stage *hemi.Stage, req *msgx.Message) {
		stage.Quit() // blocking
		os.Exit(0)
	},
	comdReconf: func(stage *hemi.Stage, req *msgx.Message) {
		if newStage, err := hemi.ApplyFile(configBase, configFile); err == nil {
			id := stage.ID() + 1
			newStage.Start(id)
			currentStage = newStage
			stage.Quit()
		}
	},
	comdCPU: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfCPU()
	},
	comdHeap: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfHeap()
	},
	comdThread: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfThread()
	},
	comdGoroutine: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfGoroutine()
	},
	comdBlock: func(stage *hemi.Stage, req *msgx.Message) {
		stage.ProfBlock()
	},
	comdGC: func(stage *hemi.Stage, req *msgx.Message) {
		runtime.GC()
	},
}
