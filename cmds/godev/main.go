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

  help       # show this message
  version    # show version info
  advise     # show platform advice
  serve      # start as server

  Only one action is allowed at a time.
  If ACTION is missing, the default action is "serve".

OPTIONS
-------

  -base   <path>      # base directory of the program
  -data   <path>      # data directory to use
  -logs   <path>      # logs directory to use
  -temp   <path>      # temp directory to use
  -config <config>    # path or url to config file

  All options apply for "serve" only.

`

func main() {
	if len(os.Args) == 2 && os.Args[1] == "test" {
		test.Main()
	} else {
		manager.Main("godev", usage, manager.ProcModeDevelop, "127.0.0.1:9526")
	}
}
