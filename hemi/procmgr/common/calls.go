// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// List of calls.

package common

const (
	ComdPids   = iota // report pids of leader and worker
	ComdLeader        // report leader info
	ComdWorker        // report worker info
	ComdReload        // reload worker config
)
