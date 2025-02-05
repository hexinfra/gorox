// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Rockman client.

package leader

import (
	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/library/msgx"
)

var roxChan = make(chan *msgx.Message) // used to send messages to workerKeeper

func rockmanClient() { // runner
	hemi.Println("[leader] rockmanClient: TODO")
}
