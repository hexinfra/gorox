// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package main

import (
	"github.com/hexinfra/gorox/hemi/procman"

	_ "github.com/hexinfra/gorox/hemi/gosites/apps"
)

const usage = `
Gosites (%s)
================================================================================

  gosites [ACTION] [OPTIONS]

ACTION
------

  serve      # start as server
  check      # dry run to check config
  help       # show this message
  version    # show version info
  advise     # show how to optimize current platform
  stop       # tell server to exit immediately
  quit       # tell server to exit gracefully
  pid        # call server to report pids of leader and worker
  leader     # call leader to report its info
  rework     # tell leader to restart worker gracefully
  readmin    # tell leader to reopen its admin interface
  ping       # call leader to give a pong
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

  "-target" applies for telling and calling actions only.
  "-admin" and "-myrox" apply for "serve" and "readmin".
  Other options apply for "serve" only.

`

func main() {
	procman.Main("gosites", usage, 0, "127.0.0.1:9525")
}
