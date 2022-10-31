// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Derox server (leader & worker(s)) and its control agent.

package main

import (
	_ "github.com/hexinfra/gorox/cmds/derox/apps/derox"
	_ "github.com/hexinfra/gorox/cmds/derox/svcs/derox"
)

import (
	_ "github.com/hexinfra/gorox/hemi/contrib/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/contrib/cachers/redis"
	_ "github.com/hexinfra/gorox/hemi/contrib/changers/gunzip"
	_ "github.com/hexinfra/gorox/hemi/contrib/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/contrib/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/contrib/filters/tcps/mysql"
	_ "github.com/hexinfra/gorox/hemi/contrib/filters/tcps/redis"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/access"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/ajp"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/favicon"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/gatex"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/hello"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/hostname"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/https"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/limit"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/referer"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/rewrite"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/sitex"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/uwsgi"
	_ "github.com/hexinfra/gorox/hemi/contrib/optwares/demo"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/replace"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/ssi"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/contrib/runners/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/contrib/runners/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/contrib/runners/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/contrib/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/contrib/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/contrib/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/contrib/staters/local"
	_ "github.com/hexinfra/gorox/hemi/contrib/staters/redis"
)

import "github.com/hexinfra/gorox/hemi/manager"

const usage = `
Derox (%s)
================================================================================

  derox [ACTION] [OPTIONS]

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

  -target <addr>      # leader address to tell or call (default: 127.0.0.1:9526)
  -admin  <addr>      # listen address of leader admin (default: 127.0.0.1:9526)
  -try                # try to run server with config
  -base   <path>      # base directory of the program
  -data   <path>      # data directory to use
  -logs   <path>      # logs directory to use
  -temp   <path>      # temp directory to use
  -config <config>    # path or url to worker config file
  -log    <path>      # leader log file (default: derox-leader.log in logs dir)
  -user   <user>      # user for worker(s) (default: nobody)
  -multi  <number>    # run server in multi-worker mode (default: 0, means no)
  -pin                # pin cpu in multi-worker mode (default: false)
  -daemon             # run server as daemon (default: false)

  "-target" applies for telling and calling actions only.
  "-admin" applies for "reopen" and "serve".
  Other options apply for "serve" only.

`

func main() {
	manager.Main("derox", usage, manager.ProcModeDevelop, "127.0.0.1:9526")
}
