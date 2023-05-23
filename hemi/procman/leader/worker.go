// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Worker keeper.

package leader

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/common/system"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

var keeperChan chan chan *msgx.Message

func workerKeeper(base string, file string) { // goroutine
	dieChan := make(chan int) // dead worker go through this channel

	rand.Seed(time.Now().UnixNano())
	const chars = "0123456789"
	keyBuffer := make([]byte, 32)
	for i := 0; i < len(keyBuffer); i++ {
		keyBuffer[i] = chars[rand.Intn(10)]
	}
	connKey := string(keyBuffer)

	worker := newWorker(connKey)
	worker.start(base, file, dieChan)
	keeperChan <- nil // reply that we have created the worker.

	for { // each event from cmduiServer()/webuiServer()/myroxClient() and worker
		select {
		case msgChan := <-keeperChan: // a message arrives
			req := <-msgChan
			if req.IsTell() {
				switch req.Comd {
				case common.ComdQuit:
					worker.tell(req)
					exitCode := <-dieChan
					os.Exit(exitCode)
				case common.ComdRework: // restart worker
					// Create new worker
					dieChan2 := make(chan int)
					worker2 := newWorker(connKey)
					worker2.start(base, file, dieChan2)
					// Quit old worker
					req.Comd = common.ComdQuit
					worker.tell(req)
					worker.reset()
					<-dieChan
					// Use new worker
					dieChan, worker = dieChan2, worker2
				default: // other messages are sent to worker
					worker.tell(req)
				}
			} else { // call
				var resp *msgx.Message
				switch req.Comd {
				case common.ComdPids:
					resp = msgx.NewMessage(req.Comd, req.Flag, nil)
					resp.Set("leader", strconv.Itoa(os.Getpid()))
					resp.Set("worker", strconv.Itoa(worker.pid()))
				default:
					resp = worker.call(req)
				}
				msgChan <- resp
			}
		case exitCode := <-dieChan: // worker process dies unexpectedly
			// TODO: more details
			if exitCode == common.CodeCrash || exitCode == common.CodeStop || exitCode == hemi.CodeBug || exitCode == hemi.CodeUse || exitCode == hemi.CodeEnv {
				fmt.Println("worker critical error")
				common.Stop()
			} else if now := time.Now(); now.Sub(worker.lastDie) > time.Second {
				worker.reset()
				worker.lastDie = now
				worker.start(base, file, dieChan) // start again
			} else { // worker has suffered too frequent crashes, unable to serve!
				fmt.Printf("worker is broken! code=%d\n", exitCode)
				common.Stop()
			}
		}
	}
}

// worker denotes the worker process
type worker struct {
	process *os.Process
	connKey string
	msgConn net.Conn
	lastDie time.Time
}

func newWorker(connKey string) *worker {
	w := new(worker)
	w.connKey = connKey
	return w
}

func (w *worker) start(base string, file string, dieChan chan int) {
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

	// Accept msgConn from worker
	msgConn, err := tmpGate.Accept()
	if err != nil {
		common.Crash(err.Error())
	}
	tmpGate.Close()

	// msgConn is established, now register worker process
	loginReq, ok := msgx.Recv(msgConn, 16<<10)
	if !ok || loginReq.Get("connKey") != w.connKey {
		common.Crash("bad worker")
	}
	if !msgx.Send(msgConn, msgx.NewMessage(loginReq.Comd, loginReq.Flag, map[string]string{
		"base": base,
		"file": file,
	})) {
		common.Crash("send worker")
	}

	// Register succeed, save msgConn and start waiting
	w.msgConn = msgConn
	go w.watch(dieChan)
}
func (w *worker) watch(dieChan chan int) { // goroutine
	stat, err := w.process.Wait()
	if err != nil {
		common.Crash(err.Error())
	}
	dieChan <- stat.ExitCode()
}

func (w *worker) tell(req *msgx.Message) { msgx.Tell(w.msgConn, req) }
func (w *worker) call(req *msgx.Message) (resp *msgx.Message) {
	resp, ok := msgx.Call(w.msgConn, req, 16<<20)
	if !ok {
		resp = msgx.NewMessage(req.Comd, 0, nil)
		resp.Flag = 0xffff
		resp.Set("worker", "failed")
	}
	return resp
}

func (w *worker) pid() int { return w.process.Pid }

func (w *worker) reset() { w.msgConn.Close() }
