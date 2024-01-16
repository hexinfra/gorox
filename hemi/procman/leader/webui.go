// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// WebUI server.

package leader

import (
	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"
	"github.com/hexinfra/gorox/hemi/procman/common"

	_ "github.com/hexinfra/gorox/hemi/procman/leader/webui"
)

var (
	webStage *hemi.Stage
	webChan  chan *msgx.Message
)

func webuiServer() {
	webChan = make(chan *msgx.Message)
	if hemi.Debug() >= 1 {
		hemi.Printf("[leader] open webui interface: %s\n", common.WebUIAddr)
	}
	// TODO
}
