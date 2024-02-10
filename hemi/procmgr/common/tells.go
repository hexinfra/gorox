// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// List of tells.

package common

const (
	ComdStop      = iota // exit server immediately
	ComdQuit             // exit server gracefully
	ComdRecmd            // reopen cmdui interface
	ComdReweb            // reopen webui interface
	ComdRework           // restart worker process
	ComdCPU              // profile cpu
	ComdHeap             // profile heap
	ComdThread           // profile thread
	ComdGoroutine        // profile goroutine
	ComdBlock            // profile block
	ComdGC               // run runtime.GC()
)
