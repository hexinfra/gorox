// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Worker abstraction in leader.

package leader

import (
	"net"
	"os"
	"time"

	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/common/system"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

// worker denotes the worker process
type worker struct {
	process *os.Process
	connKey string
	cmdConn net.Conn
	lastDie time.Time
}

func newWorker(connKey string) *worker {
	w := new(worker)
	w.connKey = connKey
	return w
}

func (w *worker) start(base string, file string, deadWay chan int) {
	// Open temporary gate
	tmpGate, err := net.Listen("tcp", "127.0.0.1:0") // port is random
	if err != nil {
		common.Crash(err.Error())
	}

	// Create worker process
	process, err := os.StartProcess(system.ExePath, common.ExeArgs, &os.ProcAttr{
		Env:   []string{"_DAEMON_=" + tmpGate.Addr().String() + "|" + w.connKey, "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")},
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr}, // inherit standard files
		Sys:   system.DaemonSysAttr(),
	})
	if err != nil {
		common.Crash(err.Error())
	}
	w.process = process

	// Accept cmdConn from worker
	cmdConn, err := tmpGate.Accept()
	if err != nil {
		common.Crash(err.Error())
	}
	tmpGate.Close()

	// cmdConn is established, now register worker process
	loginReq, ok := msgx.Recv(cmdConn, 16<<10)
	if !ok || loginReq.Get("connKey") != w.connKey {
		common.Crash("bad worker")
	}
	if !msgx.Send(cmdConn, msgx.NewMessage(loginReq.Comd, loginReq.Flag, map[string]string{
		"base": base,
		"file": file,
	})) {
		common.Crash("send worker")
	}

	// Register succeed, save cmdConn and start waiting
	w.cmdConn = cmdConn
	go w.watch(deadWay)
}
func (w *worker) watch(deadWay chan int) { // goroutine
	stat, err := w.process.Wait()
	if err != nil {
		common.Crash(err.Error())
	}
	deadWay <- stat.ExitCode()
}

func (w *worker) tell(req *msgx.Message) { msgx.Tell(w.cmdConn, req) }
func (w *worker) call(req *msgx.Message) (resp *msgx.Message) {
	resp, ok := msgx.Call(w.cmdConn, req, 16<<20)
	if !ok {
		resp = msgx.NewMessage(req.Comd, 0, nil)
		resp.Flag = 0xffff
		resp.Set("worker", "failed")
	}
	return resp
}

func (w *worker) reset() { w.cmdConn.Close() }
