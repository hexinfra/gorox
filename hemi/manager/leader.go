// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader process of manager.

package manager

import (
	"fmt"
	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/libraries/msgx"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"time"
)

var logger *os.File // used by leader only

// leaderMain is main() for leader process.
func leaderMain() {
	// Prepare logger
	file := *logFile
	if file == "" {
		file = *logsDir + "/" + program + "-leader.log"
	} else if !filepath.IsAbs(file) {
		file = *baseDir + "/" + file
	}
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		crash(err.Error())
	}
	osFile, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0700)
	if err != nil {
		crash(err.Error())
	}
	logger = osFile

	// Load config
	prefix, file := getConfig()
	logger.WriteString("parse config\n")
	if _, err := hemi.ApplyFile(prefix, file); err != nil {
		crash("leader: " + err.Error())
	}

	// Start workers
	msgChan := make(chan *msgx.Message) // msgChan is channel between leaderMain() and keepWorkers()
	go keepWorkers(prefix, file, msgChan)
	<-msgChan // waiting for keepWorkers() to ensure all workers have started.

	// Start admin
	logger.WriteString("listen at: " + adminAddr + "\n")
	admDoor, err := net.Listen("tcp", adminAddr) // admDoor is for receiving msgConns from control agent
	if err != nil {
		crash(err.Error())
	}

	var (
		req  *msgx.Message
		resp *msgx.Message
		ok   bool
	)
	for { // each msgConn from control agent
		msgConn, err := admDoor.Accept() // msgConn is connection between leader and control agent
		if err != nil {
			continue
		}
		if msgConn.SetReadDeadline(time.Now().Add(10*time.Second)) != nil {
			goto closeNext
		}
		req, ok = msgx.RecvMessage(msgConn)
		if !ok {
			goto closeNext
		}
		if req.IsTell() {
			// Some messages are telling leader only, hijack them.
			if req.Comd == comdStop {
				logger.WriteString("received stop\n")
				stop() // worker(s) will stop immediately after the pipe is closed
			} else if req.Comd == comdReopen {
				newAddr := req.Get("newAddr") // succeeding adminAddr
				if newAddr == "" {
					goto closeNext
				}
				if newDoor, err := net.Listen("tcp", newAddr); err == nil {
					admDoor.Close()
					admDoor = newDoor
					fmt.Fprintf(logger, "reopen to %s\n", newAddr)
					goto closeNext
				} else {
					fmt.Fprintf(logger, "reopen failed: %s\n", err.Error())
				}
			} else { // the rest messages are sent to keepWorkers().
				msgChan <- req
			}
		} else { // call
			// Some messages are calling leader only, hijack them.
			if req.Comd == comdPing {
				resp := msgx.NewMessage(comdPing, req.Flag, nil)
				msgx.SendMessage(msgConn, resp)
			} else { // the rest messages are sent to keepWorkers().
				msgChan <- req
				resp = <-msgChan
				if req.Comd == comdInfo {
					resp.Set("leader", fmt.Sprintf("%d", os.Getpid()))
				}
				msgx.SendMessage(msgConn, resp)
			}
		}
	closeNext:
		msgConn.Close()
	}
}

func keepWorkers(prefix string, file string, msgChan chan *msgx.Message) {
	workMode, totalWorkers := workAlone, 1
	if *multiple != 0 { // change to multi-worker mode
		workMode, totalWorkers = workShard, *multiple
	}

	nwAlive := totalWorkers
	dieChan := make(chan *worker) // all dead workers go through this channel
	pipeKey := newPipeKey()

	workers := makeWorkers(workMode, totalWorkers, prefix, file, dieChan, pipeKey)
	msgChan <- nil // reply to leaderMain that we have created the workers.

	for { // each event from leaderMain and workers
		select {
		case req := <-msgChan: // a message arrives from leaderMain
			if req.IsTell() {
				switch req.Comd {
				case comdRework: // restart workers
					newDieChan := make(chan *worker)
					newWorkers := makeWorkers(workMode, totalWorkers, prefix, file, newDieChan, pipeKey)
					// Shutdown old workers
					req.Comd = comdQuit
					for _, worker := range workers {
						if !worker.broken {
							msgx.Tell(worker.msgPipe, req)
						}
					}
					go func(dieChan chan *worker, nWorkers int) {
						for i := 0; i < nWorkers; i++ {
							<-dieChan
						}
					}(dieChan, nwAlive)
					// Use new workers
					workers, dieChan = newWorkers, newDieChan
				case comdCPU, comdHeap, comdThread, comdGoroutine, comdBlock: // profiling
					for _, worker := range workers {
						if !worker.broken {
							msgx.Tell(worker.msgPipe, req)
							break // only profile the first alive worker
						}
					}
				default: // tell workers
					for _, worker := range workers {
						if !worker.broken {
							msgx.Tell(worker.msgPipe, req)
						}
					}
					if req.Comd == comdQuit {
						os.Exit(0)
					}
				}
			} else { // call
				resp := msgx.NewMessage(req.Comd, 0, nil)
				for _, worker := range workers {
					if worker.broken {
						continue
					}
					// worker is alive, call it.
					if workerResp, ok := msgx.Call(worker.msgPipe, req); ok {
						for name, value := range workerResp.Args {
							resp.Set(name, value)
						}
						//resp.Set(worker.name, workerResp.Get("pid"))
					} else {
						resp.Flag = 0xffff
						resp.Set(worker.name, "failed")
					}
				}
				msgChan <- resp
			}
		case worker := <-dieChan: // a worker process dies
			// TODO: more details
			if exitCode := worker.exitCode; exitCode == codeCrash || exitCode == codeStop || exitCode == hemi.CodeBug || exitCode == hemi.CodeUse || exitCode == hemi.CodeEnv {
				fmt.Println("worker critical error")
				stop()
			} else if now := time.Now(); now.Sub(worker.lastExit) > time.Second {
				worker.lastExit = now
				worker.start(prefix, file, dieChan) // start again
			} else { // worker has suffered too frequent crashes. mark it as broken.
				worker.broken = true
				nwAlive--
				if nwAlive == 0 {
					logger.WriteString("all workers are broken!\n")
					stop()
				}
			}
		}
	}
}

func makeWorkers(workMode uint16, nWorkers int, prefix string, file string, dieChan chan *worker, pipeKey string) []*worker {
	workers := make([]*worker, nWorkers)
	for id := 0; id < nWorkers; id++ {
		worker := newWorker(id, workMode, pipeKey)
		worker.start(prefix, file, dieChan)
		fmt.Fprintf(logger, "worker id=%d started\n", id)
		workers[id] = worker
	}
	return workers
}
func newPipeKey() string {
	rand.Seed(time.Now().UnixNano())
	const chars = "0123456789"
	pipeKey := make([]byte, 32)
	for i := 0; i < len(pipeKey); i++ {
		pipeKey[i] = chars[rand.Intn(10)]
	}
	return string(pipeKey)
}

// worker denotes a worker process used only in leader process
type worker struct {
	id       int    // 0, 1, ...
	idString string // "0", "1", ...
	workMode uint16 // workAlone, workShard
	name     string // "worker", "worker[0]", "worker[1]", ...
	pipeKey  string

	process  *os.Process
	msgPipe  net.Conn // msgPipe is connection between leader and worker. it is actually a tcp conn on 127.0.0.1
	exitCode int

	lastExit time.Time
	broken   bool // can't relive again if broken
}

func newWorker(id int, workMode uint16, pipeKey string) *worker {
	w := new(worker)
	w.id = id
	w.idString = fmt.Sprintf("%d", id)
	w.workMode = workMode
	if workMode == workAlone {
		w.name = "worker"
	} else {
		w.name = fmt.Sprintf("worker[%d]", id)
	}
	w.pipeKey = pipeKey
	return w
}
func (w *worker) start(prefix string, file string, dieChan chan *worker) {
	tmpGate, err := net.Listen("tcp", "127.0.0.1:0") // port is random
	if err != nil {
		crash(err.Error())
	}
	// Create worker process
	process, err := os.StartProcess(system.ExePath, procArgs, &os.ProcAttr{
		Env:   []string{"_DAEMON_=" + tmpGate.Addr().String() + "|" + w.pipeKey + "|" + w.idString, "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")},
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys:   system.DaemonSysAttr(),
	})
	if err != nil {
		crash(err.Error())
	}
	w.process = process
	if w.workMode == workShard && *pinCPU && !system.SetAffinity(process.Pid, w.id) {
		crash("set affinity failed")
	}

	// Establish pipe with worker process
	msgPipe, err := tmpGate.Accept()
	if err != nil {
		// TODO
		crash(err.Error())
	}
	tmpGate.Close() // opening duration is short

	// Worker register
	req, ok := msgx.RecvMessage(msgPipe)
	if !ok || req.Get("pipeKey") != w.pipeKey || req.Get("workerID") != w.idString {
		crash("bad worker")
	}
	resp := msgx.NewMessage(req.Comd, req.Flag, map[string]string{
		"prefix": prefix,
		"file":   file,
	})
	msgx.SendMessage(msgPipe, resp)
	w.msgPipe = msgPipe

	// Tell worker process to start serve
	msgx.Tell(w.msgPipe, msgx.NewMessage(comdRun, w.workMode, nil))

	// Watch process
	go w.watch(dieChan)
}
func (w *worker) watch(dieChan chan *worker) {
	stat, err := w.process.Wait()
	if err != nil {
		crash(err.Error())
	}
	w.msgPipe.Close()
	w.exitCode = stat.ExitCode()
	dieChan <- w
}
