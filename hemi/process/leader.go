// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader process.

// Some terms:
//   admGate - Used by leader process, for receiving admConns from control agent
//   admConn - control agent ----> adminServer()
//   msgChan - adminServer()/goopsClient() <---> keepWorker()
//   deadWay - keepWorker() <---- worker.wait()
//   cmdPipe - leader process <---> worker process
//   cmcConn - leader goopsClient <---> goops

package process

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

// logger is leader's logger.
var logger *log.Logger

// leaderMain is main() for leader process.
func leaderMain() {
	// Prepare leader's logger
	logFile := *logFile
	if logFile == "" {
		logFile = *logsDir + "/" + progName + "-leader.log"
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

	if *goopsAddr == "" {
		adminServer()
	} else {
		goopsClient()
	}
}

func adminServer() {
	// Load worker's config
	base, file := getConfig()
	logger.Printf("parse worker config: base=%s file=%s\n", base, file)
	if _, err := hemi.ApplyFile(base, file); err != nil {
		crash("leader: " + err.Error())
	}

	// Start the worker
	msgChan := make(chan *msgx.Message) // msgChan is the channel between leaderMain() and keepWorker()
	go keepWorker(base, file, msgChan)
	<-msgChan // wait for keepWorker() to ensure worker is started.
	logger.Println("worker process started")

	logger.Printf("open admin interface: %s\n", adminAddr)
	admGate, err := net.Listen("tcp", adminAddr) // admGate is for receiving admConns from control agent
	if err != nil {
		crash(err.Error())
	}
	var (
		req *msgx.Message
		ok  bool
	)
	for { // each admConn from control agent
		admConn, err := admGate.Accept() // admConn is connection between leader and control agent
		if err != nil {
			logger.Println(err.Error())
			continue
		}
		if err := admConn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			logger.Println(err.Error())
			goto closeNext
		}
		req, ok = msgx.Recv(admConn, 16<<20)
		if !ok {
			goto closeNext
		}
		logger.Printf("received from agent: %v\n", req)
		if req.IsTell() {
			// Some messages are telling leader only, hijack them.
			if req.Comd == comdStop {
				logger.Println("received stop")
				stop() // worker will stop immediately after the pipe is closed
			} else if req.Comd == comdReopen {
				newAddr := req.Get("newAddr") // succeeding adminAddr
				if newAddr == "" {
					goto closeNext
				}
				if newGate, err := net.Listen("tcp", newAddr); err == nil {
					admGate.Close()
					admGate = newGate
					logger.Printf("reopen to %s\n", newAddr)
					goto closeNext
				} else {
					logger.Printf("reopen failed: %s\n", err.Error())
				}
			} else { // other messages are sent to keepWorker().
				msgChan <- req
			}
		} else { // call
			var resp *msgx.Message
			// Some messages are calling leader only, hijack them.
			if req.Comd == comdPing {
				resp = msgx.NewMessage(comdPing, req.Flag, nil)
				resp.Set(fmt.Sprintf("leader=%d", os.Getpid()), "pong")
			} else { // other messages are sent to keepWorker().
				msgChan <- req
				resp = <-msgChan
			}
			logger.Printf("send response: %v\n", resp)
			msgx.Send(admConn, resp)
		}
	closeNext:
		admConn.Close()
	}
}
func goopsClient() {
	// TODO
	fmt.Println("TODO")
}

func keepWorker(base string, file string, msgChan chan *msgx.Message) { // goroutine
	deadWay := make(chan int) // dead worker go through this channel

	rand.Seed(time.Now().UnixNano())
	const chars = "0123456789"
	keyBuffer := make([]byte, 32)
	for i := 0; i < len(keyBuffer); i++ {
		keyBuffer[i] = chars[rand.Intn(10)]
	}
	pipeKey := string(keyBuffer)

	worker := newWorker(pipeKey)
	worker.start(base, file, deadWay)
	msgChan <- nil // reply to leaderMain that we have created the worker.

	for { // each event from leaderMain and worker
		select {
		case req := <-msgChan: // a message arrives from leaderMain
			if req.IsTell() {
				switch req.Comd {
				case comdQuit:
					worker.tell(req)
					exitCode := <-deadWay
					os.Exit(exitCode)
				case comdRework: // restart worker
					// Create new worker
					deadWay2 := make(chan int)
					worker2 := newWorker(pipeKey)
					worker2.start(base, file, deadWay2)
					// Quit old worker
					req.Comd = comdQuit
					worker.tell(req)
					worker.reset()
					<-deadWay
					// Use new worker
					deadWay, worker = deadWay2, worker2
				default: // tell worker
					worker.tell(req)
				}
			} else { // call
				msgChan <- worker.call(req)
			}
		case exitCode := <-deadWay: // worker process dies unexpectedly
			// TODO: more details
			if exitCode == codeCrash || exitCode == codeStop || exitCode == hemi.CodeBug || exitCode == hemi.CodeUse || exitCode == hemi.CodeEnv {
				logger.Println("worker critical error")
				stop()
			} else if now := time.Now(); now.Sub(worker.lastDie) > time.Second {
				worker.reset()
				worker.lastDie = now
				worker.start(base, file, deadWay) // start again
			} else { // worker has suffered too frequent crashes, unable to serve!
				logger.Println("worker is broken!")
				stop()
			}
		}
	}
}

// worker denotes the worker process used only in leader process
type worker struct {
	pipeKey string
	process *os.Process
	cmdPipe net.Conn
	lastDie time.Time
}

func newWorker(pipeKey string) *worker {
	w := new(worker)
	w.pipeKey = pipeKey
	return w
}

func (w *worker) start(base string, file string, deadWay chan int) {
	// Open temporary gate
	tmpGate, err := net.Listen("tcp", "127.0.0.1:0") // port is random
	if err != nil {
		crash(err.Error())
	}

	// Create worker process
	process, err := os.StartProcess(system.ExePath, procArgs, &os.ProcAttr{
		Env:   []string{"_DAEMON_=" + tmpGate.Addr().String() + "|" + w.pipeKey, "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")},
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr}, // inherit
		Sys:   system.DaemonSysAttr(),
	})
	if err != nil {
		crash(err.Error())
	}
	w.process = process

	// Accept pipe from worker
	cmdPipe, err := tmpGate.Accept()
	if err != nil {
		crash(err.Error())
	}
	tmpGate.Close()

	// Pipe is established, now register worker process
	loginReq, ok := msgx.Recv(cmdPipe, 16<<10)
	if !ok || loginReq.Get("pipeKey") != w.pipeKey {
		crash("bad worker")
	}
	if !msgx.Send(cmdPipe, msgx.NewMessage(loginReq.Comd, loginReq.Flag, map[string]string{
		"base": base,
		"file": file,
	})) {
		crash("send worker")
	}

	// Register succeed, save pipe and start waiting
	w.cmdPipe = cmdPipe
	go w.watch(deadWay)
}
func (w *worker) watch(deadWay chan int) { // goroutine
	stat, err := w.process.Wait()
	if err != nil {
		crash(err.Error())
	}
	deadWay <- stat.ExitCode()
}

func (w *worker) tell(req *msgx.Message) {
	msgx.Tell(w.cmdPipe, req)
}
func (w *worker) call(req *msgx.Message) (resp *msgx.Message) {
	resp, ok := msgx.Call(w.cmdPipe, req, 16<<20)
	if !ok {
		resp = msgx.NewMessage(req.Comd, 0, nil)
		resp.Flag = 0xffff
		resp.Set("worker", "failed")
	}
	return resp
}

func (w *worker) reset() {
	w.cmdPipe.Close()
}
