// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// List of tells.

package common

const (
	ComdServe     = iota // start server. no tell action is bound to this comd. must be 0
	ComdStop             // exit server immediately
	ComdQuit             // exit server gracefully
	ComdRework           // restart worker process
	ComdReadmin          // reopen admin interface
	ComdReload           // reload config
	ComdCPU              // profile cpu
	ComdHeap             // profile heap
	ComdThread           // profile thread
	ComdGoroutine        // profile goroutine
	ComdBlock            // profile block
	ComdGC               // run runtime.GC()
)
