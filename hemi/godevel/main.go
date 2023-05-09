// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Godevel server (leader & worker) and its control client.

package main

import (
	"os"

	"github.com/hexinfra/gorox/hemi/godevel/test"
	"github.com/hexinfra/gorox/hemi/procman"

	_ "github.com/hexinfra/gorox/hemi/godevel/apps"
	_ "github.com/hexinfra/gorox/hemi/godevel/exts"
	_ "github.com/hexinfra/gorox/hemi/godevel/jobs"
	_ "github.com/hexinfra/gorox/hemi/godevel/srvs"
	_ "github.com/hexinfra/gorox/hemi/godevel/svcs"
)

const usage = `
Godevel (%s)
================================================================================

  godevel [ACTION] [OPTIONS]

ACTION
------

  serve        # start as server
  test         # run as tester
  help         # show this message
  version      # show version info
  advise       # show how to optimize current platform
  stop         # tell server to exit immediately
  quit         # tell server to exit gracefully
  pid          # call server to report pids of leader and worker
  leader       # call leader to report its info
  rework       # tell leader to restart worker gracefully
  reopen       # tell leader to reopen its admin interface
  ping         # call leader to give a pong
  worker       # call worker to report its info
  reload       # tell worker to reload config
  cpu          # tell worker to perform cpu profiling
  heap         # tell worker to perform heap profiling
  thread       # tell worker to perform thread profiling
  goroutine    # tell worker to perform goroutine profiling
  block        # tell worker to perform block profiling

  Only one action is allowed at a time.
  If ACTION is missing, the default action is "serve".

OPTIONS
-------

  -target <addr>      # leader address to tell or call (default: 127.0.0.1:9526)
  -admin  <addr>      # listen address of leader admin (default: 127.0.0.1:9526)
  -myrox  <addr>      # myrox address to join. if set, "-admin" will be ignored
  -try                # try to serve with config
  -single             # run server in single mode. only a process is started
  -daemon             # run server as daemon (default: false)
  -log    <path>      # leader log file (default: godevel-leader.log in logs dir)
  -base   <path>      # base directory of the program
  -logs   <path>      # logs directory to use
  -temp   <path>      # temp directory to use
  -vars   <path>      # vars directory to use
  -config <config>    # path or url to worker config file

  "-target" applies for telling and calling actions only.
  "-admin" applies for "reopen" and "serve".
  Other options apply for "serve" only.

`

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "test" {
		test.Main()
	} else {
		procman.Main("godevel", usage, 2, "127.0.0.1:9526")
	}
}
