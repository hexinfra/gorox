// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gosites server (leader & worker) and its control client.

package main

import (
	"os"

	"github.com/hexinfra/gorox/hemi/gosites/test"
	"github.com/hexinfra/gorox/hemi/procman"

	_ "github.com/hexinfra/gorox/hemi/gosites/apps"
	_ "github.com/hexinfra/gorox/hemi/gosites/exts"
	_ "github.com/hexinfra/gorox/hemi/gosites/jobs"
	_ "github.com/hexinfra/gorox/hemi/gosites/srvs"
	_ "github.com/hexinfra/gorox/hemi/gosites/svcs"
)

const usage = `
Gosites (%s)
================================================================================

  gosites [ACTION] [OPTIONS]

ACTION
------

  serve      # start as server
  check      # dry run to check config
  test       # run as tester
  help       # show this message
  version    # show version info
  advise     # show how to optimize current platform
  pids       # call server to report pids of leader and worker
  stop       # tell server to exit immediately
  quit       # tell server to exit gracefully
  leader     # call leader to report its info
  rework     # tell leader to restart worker gracefully
  readmin    # tell leader to reopen its admin interface
  worker     # call worker to report its info
  reload     # tell worker to reload config
  cpu        # tell worker to perform cpu profiling
  heap       # tell worker to perform heap profiling
  thread     # tell worker to perform thread profiling
  goroutine  # tell worker to perform goroutine profiling
  block      # tell worker to perform block profiling

  Only one action is allowed at a time.
  If ACTION is not specified, the default action is "serve".

OPTIONS
-------

  -debug  <level>   # debug level (default: 0, means disable. max: 2)
  -target <addr>    # leader address to tell or call (default: 127.0.0.1:9525)
  -admin  <addr>    # listen address of leader admin (default: 127.0.0.1:9525)
  -myrox  <addr>    # myrox address to join. if set, "-admin" will be ignored
  -conf   <config>  # path or url to worker config file
  -single           # run server in single mode. only a process is started
  -daemon           # run server as daemon (default: false)
  -log    <path>    # leader log file (default: gosites-leader.log in logs dir)
  -base   <path>    # base directory of the program
  -logs   <path>    # logs directory to use
  -temp   <path>    # temp directory to use
  -vars   <path>    # vars directory to use

  "-debug" applies to all actions.
  "-target" applies to telling and calling actions only.
  "-admin" and "-myrox" apply to "serve" and "readmin".
  Other options apply to "serve" only.

`

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "test" {
		test.Main()
	} else {
		procman.Main("gosites", usage, 0, "127.0.0.1:9525")
	}
}
