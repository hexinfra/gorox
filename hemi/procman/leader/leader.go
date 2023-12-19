// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Leader process.

package leader

import (
	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"
)

func Main() {
	// Check worker's config
	configBase, configFile := common.GetConfig()
	if hemi.Debug() >= 1 {
		hemi.Printf("[leader] check worker configBase=%s configFile=%s\n", configBase, configFile)
	}
	if _, err := hemi.BootFile(configBase, configFile); err != nil {
		common.Crash("leader: " + err.Error())
	}

	// Start the worker
	keeperChan = make(chan chan *msgx.Message)
	go workerKeeper(configBase, configFile)
	<-keeperChan // wait for workerKeeper() to ensure worker is started.

	if common.MyroxAddr != "" {
		go myroxClient()
	} else {
		// TODO: sync
		go webuiServer()
		go cmduiServer()
	}

	select {} // waiting forever
}
