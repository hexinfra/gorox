// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gorox server (leader & worker(s)) and its control agent.

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
  reopen       # tell leader to re-listen its admin interface
  rework       # tell leader to restart worker(s) gracefully
  cpu          # tell a worker to perform cpu profiling
  heap         # tell a worker to perform heap profiling
  thread       # tell a worker to perform thread profiling
  goroutine    # tell a worker to perform goroutine profiling
  block        # tell a worker to perform block profiling
  ping         # call ping of leader
  info         # call info of leader and worker(s)
  reconf       # call worker(s) to reconfigure
  serve        # start as server

  Only one action is allowed at a time.
  If ACTION is missing, the default action is "serve".

OPTIONS
-------

  -debug  <level>     # debug level (default: 0, means disable)
  -target <addr>      # leader address to tell or call (default: 127.0.0.1:9527)
  -admin  <addr>      # listen address of leader admin (default: 127.0.0.1:9527)
  -devel              # run server in developer mode. only a process is started
  -try                # try to run server with config
  -base   <path>      # base directory of the program
  -data   <path>      # data directory to use
  -logs   <path>      # logs directory to use
  -temp   <path>      # temp directory to use
  -config <config>    # path or url to worker config file
  -log    <path>      # leader log file (default: gorox-leader.log in logs dir)
  -user   <user>      # user for worker(s) (default: nobody)
  -multi  <number>    # run server in multi-worker mode (default: 0, means no)
  -pin                # pin cpu in multi-worker mode (default: false)
  -daemon             # run server as daemon (default: false)

  "-debug" applies for all actions.
  "-target" applies for telling and calling actions only.
  "-admin" applies for "reopen" and "serve".
  Other options apply for "serve" only.

`

func main() {
	manager.Main("gorox", usage, manager.ProcModeGeneral, "127.0.0.1:9527")
}
