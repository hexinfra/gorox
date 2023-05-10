// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader process.

// Some terms:
//   admGate - Used by leader process, for receiving admConns from control client
//   admConn - control client ----> adminServer()
//   msgChan - adminServer()/myroxClient() <---> keepWorker()
//   deadWay - keepWorker() <---- worker.wait()
//   cmdConn - leader process <---> worker process
//   roxConn - leader myroxClient <---> myrox

package procman

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/common/system"
)

// booker is used by leader only.
var booker *log.Logger

// leaderMain is main() for leader process.
func leaderMain() {
	// Prepare leader's booker
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
	booker = log.New(osFile, "", log.Ldate|log.Ltime)

	if *myroxAddr == "" {
		adminServer()
	} else {
		myroxClient()
	}
}

func adminServer() {
	// Load worker's config
	base, file := getConfig()
	booker.Printf("parse worker config: base=%s file=%s\n", base, file)
	if _, err := hemi.ApplyFile(base, file); err != nil {
		crash("leader: " + err.Error())
	}

	// Start the worker
	msgChan := make(chan *msgx.Message) // msgChan is the channel between leaderMain() and keepWorker()
	go keepWorker(base, file, msgChan)
	<-msgChan // wait for keepWorker() to ensure worker is started.
	booker.Println("worker process started")

	booker.Printf("open admin interface: %s\n", adminAddr)
	admGate, err := net.Listen("tcp", adminAddr) // admGate is for receiving admConns from control client
	if err != nil {
		crash(err.Error())
	}
	var (
		req *msgx.Message
		ok  bool
	)
	for { // each admConn from control client
		admConn, err := admGate.Accept() // admConn is connection between leader and control client
		if err != nil {
			booker.Println(err.Error())
			continue
		}
		if err := admConn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			booker.Println(err.Error())
			goto closeNext
		}
		req, ok = msgx.Recv(admConn, 16<<20)
		if !ok {
			goto closeNext
		}
		booker.Printf("received from client: %v\n", req)
		if req.IsTell() {
			switch req.Comd { // some messages are telling leader only, hijack them.
			case comdStop:
				booker.Println("received stop")
				stop() // worker will stop immediately after cmdConn is closed
			case comdReadmin:
				newAddr := req.Get("newAddr") // succeeding adminAddr
				if newAddr == "" {
					goto closeNext
				}
				if newGate, err := net.Listen("tcp", newAddr); err == nil {
					admGate.Close()
					admGate = newGate
					booker.Printf("admin re-opened to %s\n", newAddr)
					goto closeNext
				} else {
					booker.Printf("readmin failed: %s\n", err.Error())
				}
			default: // other messages are sent to keepWorker().
				msgChan <- req
			}
		} else { // call
			var resp *msgx.Message
			switch req.Comd { // some messages also call leader, hijack them.
			case comdPing:
				resp = msgx.NewMessage(comdPing, req.Flag, nil)
				resp.Set(fmt.Sprintf("leader=%d", os.Getpid()), "pong")
			case comdPid:
				msgChan <- req
				resp = <-msgChan
				resp.Set("leader", strconv.Itoa(os.Getpid()))
			case comdLeader:
				resp = msgx.NewMessage(comdLeader, req.Flag, nil)
				resp.Set("goroutines", "123") // TODO
			default: // other messages are sent to keepWorker().
				msgChan <- req
				resp = <-msgChan
			}
			booker.Printf("send response: %v\n", resp)
			msgx.Send(admConn, resp)
		}
	closeNext:
		admConn.Close()
	}
}
func myroxClient() {
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
	cmdKey := string(keyBuffer)

	worker := newWorker(cmdKey)
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
					worker2 := newWorker(cmdKey)
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
				booker.Println("worker critical error")
				stop()
			} else if now := time.Now(); now.Sub(worker.lastDie) > time.Second {
				worker.reset()
				worker.lastDie = now
				worker.start(base, file, deadWay) // start again
			} else { // worker has suffered too frequent crashes, unable to serve!
				booker.Println("worker is broken!")
				stop()
			}
		}
	}
}

// worker denotes the worker process used only in leader process
type worker struct {
	process *os.Process
	cmdKey  string
	cmdConn net.Conn
	lastDie time.Time
}

func newWorker(cmdKey string) *worker {
	w := new(worker)
	w.cmdKey = cmdKey
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
		Env:   []string{"_DAEMON_=" + tmpGate.Addr().String() + "|" + w.cmdKey, "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")},
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr}, // inherit
		Sys:   system.DaemonSysAttr(),
	})
	if err != nil {
		crash(err.Error())
	}
	w.process = process

	// Accept cmdConn from worker
	cmdConn, err := tmpGate.Accept()
	if err != nil {
		crash(err.Error())
	}
	tmpGate.Close()

	// cmdConn is established, now register worker process
	loginReq, ok := msgx.Recv(cmdConn, 16<<10)
	if !ok || loginReq.Get("cmdKey") != w.cmdKey {
		crash("bad worker")
	}
	if !msgx.Send(cmdConn, msgx.NewMessage(loginReq.Comd, loginReq.Flag, map[string]string{
		"base": base,
		"file": file,
	})) {
		crash("send worker")
	}

	// Register succeed, save cmdConn and start waiting
	w.cmdConn = cmdConn
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
	msgx.Tell(w.cmdConn, req)
}
func (w *worker) call(req *msgx.Message) (resp *msgx.Message) {
	resp, ok := msgx.Call(w.cmdConn, req, 16<<20)
	if !ok {
		resp = msgx.NewMessage(req.Comd, 0, nil)
		resp.Flag = 0xffff
		resp.Set("worker", "failed")
	}
	return resp
}

func (w *worker) reset() {
	w.cmdConn.Close()
}
