// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Myrox client.

package leader

import (
	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/library/msgx"
)

func myroxClient() { // runner
	// TODO
	roxChan := make(chan *msgx.Message)
	_ = roxChan
	hemi.Println("[leader] myroxClient: TODO")
}
