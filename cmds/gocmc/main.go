// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gorox Cluster Management Center/Console (leader & worker(s)) and its control agent.

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
=====================

  gocmc [ACTION] [OPTIONS]

ACTION
------

  help         # show this message
  version      # show version info
  advise       # show platform advice
  run          # start as server
  quit         # tell server to exit gracefully
  stop         # tell server to exit immediately
  reopen       # tell leader to re-listen its admin interface
  rework       # tell leader to restart worker(s) gracefully
  cpu          # tell worker to perform cpu profiling
  heap         # tell worker to perform heap profiling
  thread       # tell worker to perform thread profiling
  goroutine    # tell worker to perform goroutine profiling
  block        # tell worker to perform block profiling
  ping         # call ping of leader
  info         # call info of leader and worker(s)
  reconf       # call worker to reconfigure

  Only one action is allowed at a time.
  If ACTION is missing, the default action is "run".

OPTIONS
-------

  -debug              # enable debug
  -target <addr>      # leader address to tell or call (default: 127.0.0.1:9528)
  -admin  <addr>      # listen address of leader admin (default: 127.0.0.1:9528)
  -devel              # run server in developer mode. only a process is started
  -try                # try to run server with config
  -base   <path>      # base directory of the program
  -data   <path>      # data directory to use
  -logs   <path>      # logs directory to use
  -temp   <path>      # temp directory to use
  -config <config>    # path or url to worker config file
  -log    <path>      # leader log file (default: gocmc-leader.log in logs dir)
  -user   <user>      # user for worker(s) (default: nobody)
  -daemon             # run server as daemon (default: false)

  "-debug" applies for all actions.
  "-target" applies for telling and calling actions only.
  "-admin" applies for "reopen" and server.
  Other options apply for server only.

`

func main() {
	manager.Main("gocmc", usage, manager.ProcModeAlone, "127.0.0.1:9528")
}