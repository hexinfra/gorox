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
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"time"
)

var logger *log.Logger // used by leader only

// leaderMain is main() for leader process.
func leaderMain() {
	// Prepare logger
	logFile := *logFile
	if logFile == "" {
		logFile = *logsDir + "/" + program + "-leader.log"
	} else if !filepath.IsAbs(logFile) {
		logFile = *baseDir + "/" + logFile
	}
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		crash(err.Error())
	}
	osFile, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0700)
	if err != nil {
		crash(err.Error())
	}
	logger = log.New(osFile, "", log.Ldate|log.Ltime)

	// Load config
	base, file := getConfig()
	logger.Println("parse config")
	if _, err := hemi.ApplyFile(base, file); err != nil {
		crash("leader: " + err.Error())
	}

	// Start workers
	msgChan := make(chan *msgx.Message) // msgChan is channel between leaderMain() and keepWorkers()
	go keepWorkers(base, file, msgChan)
	<-msgChan // waiting for keepWorkers() to ensure all workers have started.

	// Start admin interface
	logger.Printf("listen at: %s\n", adminAddr)
	admDoor, err := net.Listen("tcp", adminAddr) // admDoor is for receiving msgConns from control agent
	if err != nil {
		crash(err.Error())
	}

	var (
		req *msgx.Message
		ok  bool
	)
	for { // each admConn from control agent
		admConn, err := admDoor.Accept() // admConn is connection between leader and control agent
		if err != nil {
			continue
		}
		if admConn.SetReadDeadline(time.Now().Add(10*time.Second)) != nil {
			goto closeNext
		}
		req, ok = msgx.RecvMessage(admConn)
		if !ok {
			goto closeNext
		}
		if req.IsTell() {
			// Some messages are telling leader only, hijack them.
			if req.Comd == comdStop {
				logger.Println("received stop")
				stop() // worker(s) will stop immediately after the pipe is closed
			} else if req.Comd == comdReadmin {
				newAddr := req.Get("newAddr") // succeeding adminAddr
				if newAddr == "" {
					goto closeNext
				}
				if newDoor, err := net.Listen("tcp", newAddr); err == nil {
					admDoor.Close()
					admDoor = newDoor
					logger.Printf("readmin to %s\n", newAddr)
					goto closeNext
				} else {
					logger.Printf("readmin failed: %s\n", err.Error())
				}
			} else { // the rest messages are sent to keepWorkers().
				msgChan <- req
			}
		} else { // call
			var resp *msgx.Message
			// Some messages are calling leader only, hijack them.
			if req.Comd == comdPing {
				resp = msgx.NewMessage(comdPing, req.Flag, nil)
			} else { // the rest messages are sent to keepWorkers().
				msgChan <- req
				resp = <-msgChan
				if req.Comd == comdInfo {
					resp.Set("leader", fmt.Sprintf("%d", os.Getpid()))
				}
			}
			msgx.SendMessage(admConn, resp)
		}
	closeNext:
		admConn.Close()
	}
}

func keepWorkers(base string, file string, msgChan chan *msgx.Message) { // goroutine
	workMode, totalWorkers := workTogether, 1
	if *multiple != 0 { // change to multi-worker mode
		workMode, totalWorkers = workIsolated, *multiple
	}

	nwAlive := totalWorkers
	dieChan := make(chan *worker) // all dead workers go through this channel

	rand.Seed(time.Now().UnixNano())
	const chars = "0123456789"
	keyBuffer := make([]byte, 32)
	for i := 0; i < len(keyBuffer); i++ {
		keyBuffer[i] = chars[rand.Intn(10)]
	}
	pipeKey := string(keyBuffer)

	workers := newWorkers(workMode, totalWorkers, base, file, dieChan, pipeKey)
	msgChan <- nil // reply to leaderMain that we have created the workers.

	for { // each event from leaderMain and workers
		select {
		case req := <-msgChan: // a message arrives from leaderMain
			if req.IsTell() {
				switch req.Comd {
				case comdQuit:
					for _, worker := range workers {
						if !worker.broken {
							msgx.Tell(worker.msgPipe, req)
						}
					}
					for i := 0; i < nwAlive; i++ {
						<-dieChan
					}
					os.Exit(0)
				case comdRework: // restart workers
					// Create new workers
					dieChan2 := make(chan *worker)
					workers2 := newWorkers(workMode, totalWorkers, base, file, dieChan2, pipeKey)
					// Shutdown old workers
					req.Comd = comdQuit
					for _, worker := range workers {
						if !worker.broken {
							msgx.Tell(worker.msgPipe, req)
						}
					}
					go func(dieChan chan *worker, nWorkers int) { // goroutine
						for i := 0; i < nWorkers; i++ {
							<-dieChan
						}
					}(dieChan, nwAlive)
					// Use new workers
					workers, dieChan = workers2, dieChan2
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
		case worker := <-dieChan: // a worker process dies unexpectedly
			// TODO: more details
			if code := worker.exitCode; code == codeCrash || code == codeStop || code == hemi.CodeBug || code == hemi.CodeUse || code == hemi.CodeEnv {
				fmt.Println("worker critical error")
				stop()
			} else if now := time.Now(); now.Sub(worker.lastExit) > 1*time.Second {
				worker.lastExit = now
				worker.start(base, file, dieChan) // start again
			} else { // worker has suffered too frequent crashes. mark it as broken.
				worker.broken = true
				nwAlive--
				if nwAlive == 0 {
					logger.Println("all workers are broken!")
					stop()
				}
			}
		}
	}
}

// worker denotes a worker process used only in leader process
type worker struct {
	id       int    // 0, 1, ...
	idString string // "0", "1", ...
	workMode uint16 // workTogether, workIsolated
	name     string // "worker", "worker[0]", "worker[1]", ...
	pipeKey  string

	process  *os.Process
	msgPipe  net.Conn // msgPipe is connection between leader and worker. it is actually a tcp conn on 127.0.0.1
	exitCode int

	lastExit time.Time
	broken   bool // can't relive again if broken
}

func newWorkers(workMode uint16, nWorkers int, base string, file string, dieChan chan *worker, pipeKey string) []*worker {
	workers := make([]*worker, nWorkers)
	for id := 0; id < nWorkers; id++ {
		worker := newWorker(id, workMode, pipeKey)
		worker.start(base, file, dieChan)
		logger.Printf("worker id=%d started\n", id)
		workers[id] = worker
	}
	return workers
}
func newWorker(id int, workMode uint16, pipeKey string) *worker {
	w := new(worker)
	w.id = id
	w.idString = fmt.Sprintf("%d", id)
	w.workMode = workMode
	if workMode == workTogether {
		w.name = "worker"
	} else {
		w.name = fmt.Sprintf("worker[%d]", id)
	}
	w.pipeKey = pipeKey
	return w
}
func (w *worker) start(base string, file string, dieChan chan *worker) {
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
	if w.workMode == workIsolated && *pinCPU && !system.SetAffinity(process.Pid, w.id) {
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
	loginReq, ok := msgx.RecvMessage(msgPipe)
	if !ok || loginReq.Get("pipeKey") != w.pipeKey || loginReq.Get("workerID") != w.idString {
		crash("bad worker")
	}
	loginResp := msgx.NewMessage(loginReq.Comd, loginReq.Flag, map[string]string{
		"base": base,
		"file": file,
	})
	msgx.SendMessage(msgPipe, loginResp)
	w.msgPipe = msgPipe

	// Register succeed, now tell worker process to start serve
	msgx.Tell(w.msgPipe, msgx.NewMessage(comdServe, w.workMode, nil))

	// Watch process
	go w.watch(dieChan)
}
func (w *worker) watch(dieChan chan *worker) { // goroutine
	stat, err := w.process.Wait()
	if err != nil {
		crash(err.Error())
	}
	w.msgPipe.Close()
	w.exitCode = stat.ExitCode()
	dieChan <- w
}
