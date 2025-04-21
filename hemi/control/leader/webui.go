// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// WebUI server.

package leader

import (
	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/control/common"
	"github.com/hexinfra/gorox/hemi/library/msgx"

	_ "github.com/hexinfra/gorox/hemi/control/leader/webui"
)

var webChan = make(chan *msgx.Message) // used to send messages to workerKeeper

func webuiServer() { // runner
	if hemi.DebugLevel() >= 1 {
		hemi.Printf("[leader] open webui interface: %s\n", common.WebUIAddr)
	}
	// TODO
	//webStage := hemi.StageFromText()
}
