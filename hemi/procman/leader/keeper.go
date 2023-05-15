// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Worker keeper.

package leader

import (
	"math/rand"
	"os"
	"time"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

func workerKeeper(base string, file string, msgChan chan *msgx.Message) { // goroutine
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
	msgChan <- nil // reply that we have created the worker.

	for { // each event from cmduiServer()/webuiServer()/myroxClient() and worker
		select {
		case req := <-msgChan: // a message arrives
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
				resp := worker.call(req)
				msgChan <- resp
			}
		case exitCode := <-dieChan: // worker process dies unexpectedly
			// TODO: more details
			if exitCode == common.CodeCrash || exitCode == common.CodeStop || exitCode == hemi.CodeBug || exitCode == hemi.CodeUse || exitCode == hemi.CodeEnv {
				logger.Println("worker critical error")
				common.Stop()
			} else if now := time.Now(); now.Sub(worker.lastDie) > time.Second {
				worker.reset()
				worker.lastDie = now
				worker.start(base, file, dieChan) // start again
			} else { // worker has suffered too frequent crashes, unable to serve!
				logger.Println("worker is broken!")
				common.Stop()
			}
		}
	}
}
