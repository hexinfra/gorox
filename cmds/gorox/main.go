// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gorox server (leader & worker) and its control agent.

package main

import (
	"os"
)

import (
	_ "github.com/hexinfra/gorox/apps"
	_ "github.com/hexinfra/gorox/exts"
	_ "github.com/hexinfra/gorox/svcs"
	"github.com/hexinfra/gorox/test"
)

import "github.com/hexinfra/gorox/hemi/manager"

const usage = `
Gorox (%s)
================================================================================

  gorox [ACTION] [OPTIONS]

ACTION
------

  help         # show this message
  version      # show version info
  advise       # show how to optimize current platform
  test         # run as tester
  serve        # start as server
  stop         # tell server to exit immediately
  quit         # tell server to exit gracefully
  reconf       # tell worker to reconfigure
  rework       # tell leader to restart worker gracefully
  reopen       # tell leader to reopen its admin interface
  ping         # call ping of leader
  info         # call info of server
  cpu          # tell worker to perform cpu profiling
  heap         # tell worker to perform heap profiling
  thread       # tell worker to perform thread profiling
  goroutine    # tell worker to perform goroutine profiling
  block        # tell worker to perform block profiling

  Only one action is allowed at a time.
  If ACTION is missing, the default action is "serve".

OPTIONS
-------

  -debug  <level>     # debug level (default: 0, means disable. max: 2)
  -target <addr>      # leader address to tell or call (default: 127.0.0.1:9527)
  -admin  <addr>      # listen address of leader admin (default: 127.0.0.1:9527)
  -goops  <addr>      # goops address to join. if set, "-admin" will be ignored
  -try                # try to serve with config
  -single             # run server in single mode. only a process is started
  -daemon             # run server as daemon (default: false)
  -log    <path>      # leader log file (default: gorox-leader.log in logs dir)
  -base   <path>      # base directory of the program
  -data   <path>      # data directory to use
  -logs   <path>      # logs directory to use
  -temp   <path>      # temp directory to use
  -config <config>    # path or url to worker config file

  "-debug" applies for all actions.
  "-target" applies for telling and calling actions only.
  "-admin" applies for "reopen" and "serve".
  Other options apply for "serve" only.

`

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "test" {
		test.Main()
	} else {
		manager.Main("gorox", usage, 0, "127.0.0.1:9527")
	}
}
