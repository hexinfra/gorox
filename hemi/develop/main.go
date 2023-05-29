// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Develop server (leader & worker) and its control client.

package main

import (
	"os"

	"github.com/hexinfra/gorox/hemi/develop/test"
	"github.com/hexinfra/gorox/hemi/procman"

	_ "github.com/hexinfra/gorox/hemi/develop/apps"
	_ "github.com/hexinfra/gorox/hemi/develop/exts"
	_ "github.com/hexinfra/gorox/hemi/develop/jobs"
	_ "github.com/hexinfra/gorox/hemi/develop/srvs"
	_ "github.com/hexinfra/gorox/hemi/develop/svcs"
)

const usage = `
Develop (%s)
================================================================================

  develop [ACTION] [OPTIONS]

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
  recmd      # tell leader to reopen its cmdui interface
  reweb      # tell leader to reopen its webui interface
  rework     # tell leader to restart worker gracefully
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

  -debug  <level>   # debug level (default: 2. min: 0)
  -target <addr>    # leader address to tell or call (default: 127.0.0.1:9523)
  -cmdui  <addr>    # listen address of leader cmdui (default: 127.0.0.1:9523)
  -webui  <addr>    # listen address of leader webui (default: 127.0.0.1:9524)
  -myrox  <addr>    # myrox to use. "-cmdui" and "-webui" will be ignored if set
  -config <config>  # path or url to worker config file
  -single           # run server in single mode. only a process is started
  -daemon           # run server as daemon (default: false)
  -base   <path>    # base directory of the program
  -logs   <path>    # logs directory to use
  -temp   <path>    # temp directory to use
  -vars   <path>    # vars directory to use
  -out    <path>    # daemon's stdout file (default: develop.out in logs dir)
  -err    <path>    # daemon's stderr file (default: develop.err in logs dir)

  "-debug" applies to all actions.
  "-target" applies to telling and calling actions only.
  "-cmdui" apply to "serve" and "recmd".
  "-webui" apply to "serve" and "reweb".
  Other options apply to "serve" only.

`

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "test" {
		test.Main()
	} else {
		procman.Main("develop", usage, 2, "127.0.0.1:9523", "127.0.0.1:9524")
	}
}
