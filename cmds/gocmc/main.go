// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gorox Cluster Management Center/Console (leader & worker) and its control agent.

package main

import (
	_ "github.com/hexinfra/gorox/cmds/gocmc/admin"
	_ "github.com/hexinfra/gorox/cmds/gocmc/board"
	_ "github.com/hexinfra/gorox/cmds/gocmc/iface"
	_ "github.com/hexinfra/gorox/cmds/gocmc/store"
)

import "github.com/hexinfra/gorox/hemi/manager"

const usage = `
Gocmc (%s)
================================================================================

  gocmc [ACTION] [OPTIONS]

ACTION
------

  help         # show this message
  version      # show version info
  advise       # show how to optimize current platform
  stop         # tell server to exit immediately
  quit         # tell server to exit gracefully
  rework       # tell leader to restart worker gracefully
  readmin      # tell leader to reopen its admin interface
  cpu          # tell worker to perform cpu profiling
  heap         # tell worker to perform heap profiling
  thread       # tell worker to perform thread profiling
  goroutine    # tell worker to perform goroutine profiling
  block        # tell worker to perform block profiling
  ping         # call ping of leader
  info         # call info of server
  reconf       # call worker to reconfigure
  serve        # start as server

  Only one action is allowed at a time.
  If ACTION is missing, the default action is "serve".

OPTIONS
-------

  -debug  <level>     # debug level (default: 0, means disable. max: 2)
  -target <addr>      # leader address to tell or call (default: 127.0.0.1:9528)
  -admin  <addr>      # listen address of leader admin (default: 127.0.0.1:9528)
  -single             # run server in single mode. only a process is started
  -daemon             # run server as daemon (default: false)
  -try                # try to run server with config
  -log    <path>      # leader log file (default: gocmc-leader.log in logs dir)
  -base   <path>      # base directory of the program
  -logs   <path>      # logs directory to use
  -temp   <path>      # temp directory to use
  -vars   <path>      # vars directory to use
  -config <config>    # path or url to worker config file
  -user   <user>      # user for worker (default: nobody)

  "-debug" applies for all actions.
  "-target" applies for telling and calling actions only.
  "-admin" applies for "readmin" and "serve".
  Other options apply for "serve" only.

`

func main() {
	manager.Main("gocmc", usage, false, "127.0.0.1:9528")
}
