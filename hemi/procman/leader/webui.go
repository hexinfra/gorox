// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// WebUI server.

package leader

import (
	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/msgx"

	_ "github.com/hexinfra/gorox/hemi/procman/leader/webui"
)

var (
	webChan chan *msgx.Message
	webStage *hemi.Stage
)

func webuiServer() {
	// TODO
	webChan = make(chan *msgx.Message)
}
