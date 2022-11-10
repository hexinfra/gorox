// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Godev server (leader & worker(s)) and its control agent.

package main

import (
	"os"
)

import (
	_ "github.com/hexinfra/gorox/cmds/godev/apps/test"
	_ "github.com/hexinfra/gorox/cmds/godev/exts"
	_ "github.com/hexinfra/gorox/cmds/godev/svcs/test"
	"github.com/hexinfra/gorox/cmds/godev/test"
)

import "github.com/hexinfra/gorox/hemi/manager"

const usage = `
Godev (%s)
================================================================================

  godev [ACTION] [OPTIONS]

ACTION
------

  help         # show this message
  version      # show version info
  advise       # show platform advice
  test         # run as tester
  quit         # tell server to exit gracefully
  stop         # tell server to exit immediately
  readmin      # tell leader to re-listen its admin interface
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

  -target <addr>      # leader address to tell or call (default: 127.0.0.1:9526)
  -admin  <addr>      # listen address of leader admin (default: 127.0.0.1:9526)
  -single             # run server in single mode. only a process is started
  -try                # try to run server with config
  -base   <path>      # base directory of the program
  -logs   <path>      # logs directory to use
  -temp   <path>      # temp directory to use
  -vars   <path>      # vars directory to use
  -config <config>    # path or url to worker config file
  -log    <path>      # leader log file (default: godev-leader.log in logs dir)
  -user   <user>      # user for worker(s) (default: nobody)
  -daemon             # run server as daemon (default: false)

  "-target" applies for telling and calling actions only.
  "-admin" applies for "readmin" and "serve".
  Other options apply for "serve" only.

`

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "test" {
		test.Main()
	} else {
		manager.Main("godev", usage, manager.ProcFlagDevelop, "127.0.0.1:9526")
	}
}
