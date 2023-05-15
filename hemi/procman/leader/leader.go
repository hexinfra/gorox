// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader process.

// cmdConn: control client ----> cmduiServer()
// cmdGate: used by cmduiServer(), for receiving cmdConns from control client
// webConn: control browser ----> webuiServer()
// webGate: used by webuiServer(), for receiving webConns from control browser
// roxConn: myroxClient() <---> myrox
// msgChan: cmduiServer()/webuiServer()/myroxClient() <---> keepWorker()
// deadWay: keepWorker() <---- worker.wait()
// msgConn: leader process <---> worker process

package leader

import (
	"log"
	"os"
	"path/filepath"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

var logger *log.Logger

func Main() {
	logFile := common.LogFile
	if logFile == "" {
		logFile = common.LogsDir + "/" + common.Program + "-leader.log"
	} else if !filepath.IsAbs(logFile) {
		logFile = common.BaseDir + "/" + logFile
	}
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		common.Crash(err.Error())
	}
	osFile, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0700)
	if err != nil {
		common.Crash(err.Error())
	}
	logger = log.New(osFile, "", log.Ldate|log.Ltime)

	if common.MyroxAddr == "" {
		// Load worker's config
		base, file := common.GetConfig()
		logger.Printf("parse worker config: base=%s file=%s\n", base, file)
		if _, err := hemi.ApplyFile(base, file); err != nil {
			common.Crash("leader: " + err.Error())
		}

		// Start the worker
		msgChan := make(chan *msgx.Message) // msgChan is the channel between cmduiServer()/webuiServer() and keepWorker()
		go keepWorker(base, file, msgChan)
		<-msgChan // wait for keepWorker() to ensure worker is started.
		logger.Println("worker process started")

		// TODO: msgChan MUST be protected against concurrent cmduiServer() and webuiServer()
		go cmduiServer(msgChan)
		go webuiServer(msgChan)
		select {} // waiting forever
	} else {
		myroxClient()
	}
}
