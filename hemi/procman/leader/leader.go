// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Leader process runs CmdUI server, WebUI server, Rockman client, and manages the worker process.

package leader

import (
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/library/msgx"
	"github.com/hexinfra/gorox/hemi/library/system"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

func Main() {
	// Check worker's config
	configBase, configFile := common.GetConfig()
	if hemi.DebugLevel() >= 1 {
		hemi.Printf("[leader] check worker configBase=%s configFile=%s\n", configBase, configFile)
	}
	if _, err := hemi.StageFromFile(configBase, configFile); err != nil { // the returned worker stage is ignored and not used
		common.Crash("leader: " + err.Error())
	}

	// Config check passed. Now start the worker
	go workerKeeper(configBase, configFile)
	<-keeperChan // wait for workerKeeper() to ensure worker is started.

	if common.RockmanAddr == "" {
		// TODO: sync?
		go webuiServer()
		go cmduiServer()
	} else {
		go rockmanClient()
	}

	select {} // sleep forever
}

var keeperChan = make(chan chan *msgx.Message)

func workerKeeper(configBase string, configFile string) { // runner
	dieChan := make(chan int) // dead worker goes through this channel

	rand.Seed(time.Now().UnixNano())
	const chars = "0123456789"
	keyBuffer := make([]byte, 32)
	for i := 0; i < len(keyBuffer); i++ {
		keyBuffer[i] = chars[rand.Intn(10)]
	}
	connKey := string(keyBuffer)

	worker := newWorker(connKey)
	worker.start(configBase, configFile, dieChan)
	if hemi.DebugLevel() >= 1 {
		hemi.Printf("[leader] worker process id=%d started\n", worker.pid())
	}
	keeperChan <- nil // reply to Main() that we have created the worker.

	for { // each event from cmduiServer()/webuiServer()/rockmanClient() and worker process
		select {
		case msgChan := <-keeperChan: // a message arrives
			req := <-msgChan // get the message. msgChan might be cmdChan, webChan, and roxChan
			if req.IsTell() {
				switch req.Comd {
				case common.ComdQuit:
					worker.tell(req)
					exitCode := <-dieChan
					os.Exit(exitCode)
				case common.ComdRework: // restart worker
					// Create a new worker
					dieChan2 := make(chan int)
					worker2 := newWorker(connKey)
					worker2.start(configBase, configFile, dieChan2)
					if hemi.DebugLevel() >= 1 {
						hemi.Printf("[leader] new worker process id=%d started\n", worker2.pid())
					}
					// Quit the old worker
					req.Comd = common.ComdQuit
					worker.tell(req)
					worker.closeConn()
					<-dieChan
					if hemi.DebugLevel() >= 1 {
						hemi.Printf("[leader] old worker process id=%d exited\n", worker.pid())
					}
					// Use the new worker
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
				hemi.Errorf("[leader] worker critical error! code=%d\n", exitCode)
				common.Stop()
			} else if now := time.Now(); now.Sub(worker.lastDie) > time.Second {
				hemi.Errorf("[leader] worker died unexpectedly, code=%d, restart again!\n", exitCode)
				worker.closeConn()
				worker.lastDie = now
				worker.start(configBase, configFile, dieChan) // start again
			} else { // worker has suffered too frequent crashes, unable to serve!
				hemi.Errorf("[leader] worker is broken! code=%d\n", exitCode)
				common.Stop()
			}
		}
	}
}

// worker denotes the worker process
type worker struct {
	process *os.Process
	connKey string
	admConn net.Conn // used to communicate with worker process
	lastDie time.Time
}

func newWorker(connKey string) *worker {
	w := new(worker)
	w.connKey = connKey
	return w
}

func (w *worker) start(configBase string, configFile string, dieChan chan int) {
	// Open temporary gate
	admGate, err := net.Listen("tcp", "127.0.0.1:0") // port is random
	if err != nil {
		common.Crash(err.Error())
	}

	// Create worker process
	process, err := os.StartProcess(system.ExePath, common.ProgramArgs, &os.ProcAttr{
		Env:   []string{"_GOROX_DAEMON_=" + admGate.Addr().String() + "|" + w.connKey, "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")},
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr}, // inherit standard files from leader
		Sys:   system.DaemonSysAttr(),
	})
	if err != nil {
		common.Crash(err.Error())
	}
	w.process = process

	// Accept admConn from worker
	admConn, err := admGate.Accept()
	if err != nil {
		common.Crash(err.Error())
	}

	// Close temporary gate
	admGate.Close()

	// Now that admConn is established, we register worker process
	loginReq, err := msgx.Recv(admConn, 16<<10)
	if err != nil || loginReq.Get("connKey") != w.connKey {
		common.Crash("bad worker")
	}
	if err := msgx.Send(admConn, msgx.NewMessage(loginReq.Comd, loginReq.Flag, map[string]string{
		"configBase": configBase,
		"configFile": configFile,
	})); err != nil {
		common.Crash("send worker failed: " + err.Error())
	}

	// Register succeed, save admConn and start waiting
	w.admConn = admConn
	go w.watch(dieChan)
}
func (w *worker) watch(dieChan chan int) { // runner
	stat, err := w.process.Wait()
	if err != nil {
		common.Crash(err.Error())
	}
	dieChan <- stat.ExitCode()
}

func (w *worker) tell(req *msgx.Message) { msgx.Tell(w.admConn, req) }
func (w *worker) call(req *msgx.Message) (resp *msgx.Message) {
	resp, err := msgx.Call(w.admConn, req, 16<<20)
	if err != nil {
		resp = msgx.NewMessage(req.Comd, 0, nil)
		resp.Flag = 0xffff
		resp.Set("worker", "failed")
	}
	return resp
}

func (w *worker) pid() int { return w.process.Pid }

func (w *worker) closeConn() { w.admConn.Close() }
