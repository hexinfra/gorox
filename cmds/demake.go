// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Demake builds godev only. Just a shortcut for 'gomake d'.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

const usage = `
Demake
======

  demake [OPTIONS]

OPTIONS
-------

  -fmt     # run gofmt before building
  -cgo     # enable cgo
  -race    # enable race detection
`

var (
	fmt_ = flag.Bool("fmt", false, "")
	cgo  = flag.Bool("cgo", false, "")
	race = flag.Bool("race", false, "")
)

func main() {
	flag.Usage = func() {
		fmt.Println(usage)
	}
	flag.Parse()

	if *fmt_ {
		cmd := exec.Command("gofmt", "-w", ".")
		if out, _ := cmd.CombinedOutput(); len(out) > 0 {
			fmt.Println(string(out))
			return
		}
	}
	if *cgo {
		os.Setenv("CGO_ENABLED", "1")
	} else {
		os.Setenv("CGO_ENABLED", "0")
	}

	var cmd *exec.Cmd
	if *race {
		cmd = exec.Command("go", "build", "-race", "github.com/hexinfra/gorox/cmds/godev")
	} else {
		cmd = exec.Command("go", "build", "github.com/hexinfra/gorox/cmds/godev")
	}
	fmt.Print("building godev...")
	if out, _ := cmd.CombinedOutput(); len(out) > 0 {
		fmt.Println(string(out))
	} else {
		fmt.Println("ok.")
	}
}
