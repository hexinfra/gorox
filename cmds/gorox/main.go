// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gorox server (leader & worker) and its control agent.

package main

import (
	_ "github.com/hexinfra/gorox/apps"
	_ "github.com/hexinfra/gorox/exts"
	_ "github.com/hexinfra/gorox/svcs"
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
  advise       # show platform advice
  quit         # tell server to exit gracefully
  stop         # tell server to exit immediately
  readmin      # tell leader to re-listen its admin interface
  rework       # tell leader to restart worker gracefully
  cpu          # tell worker to perform cpu profiling
  heap         # tell worker to perform heap profiling
  thread       # tell worker to perform thread profiling
  goroutine    # tell worker to perform goroutine profiling
  block        # tell worker to perform block profiling
  ping         # call ping of leader
  info         # call info of leader and worker
  reconf       # call worker to reconfigure
  serve        # start as server

  Only one action is allowed at a time.
  If ACTION is missing, the default action is "serve".

OPTIONS
-------

  -debug  <level>     # debug level (default: 0, means disable. max: 2)
  -target <addr>      # leader address to tell or call (default: 127.0.0.1:9527)
  -admin  <addr>      # listen address of leader admin (default: 127.0.0.1:9527)
  -single             # run server in single mode. only a process is started
  -daemon             # run server as daemon (default: false)
  -try                # try to run server with config
  -base   <path>      # base directory of the program
  -logs   <path>      # logs directory to use
  -temp   <path>      # temp directory to use
  -vars   <path>      # vars directory to use
  -config <config>    # path or url to worker config file
  -log    <path>      # leader log file (default: gorox-leader.log in logs dir)
  -user   <user>      # user for worker (default: nobody)

  "-debug" applies for all actions.
  "-target" applies for telling and calling actions only.
  "-admin" applies for "readmin" and "serve".
  Other options apply for "serve" only.

`

func main() {
	manager.Main("gorox", usage, false, "127.0.0.1:9527")
}
