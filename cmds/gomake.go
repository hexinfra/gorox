// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gomake builds cmds.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const usage = `
Gomake
======

  gomake [OPTIONS] [TARGET]

OPTIONS
-------

  -fmt              # run gofmt before building
  -cgo              # enable cgo
  -race             # enable race
  -os   <goos>      # target GOOS
  -arch <goarch>    # target GOARCH

  Options only take effect on building.

TARGET
------

  Specify cmd name as TARGET. If empty, the default action is: go build.
  Some special TARGETs are:

  all      # build all cmds in the directory
  clean    # clean binaries, logs, and temp files
  clear    # clear binaries, data, logs, and temp files
  dist     # make distribution
`

var (
	fmt_ = flag.Bool("fmt", false, "")
	cgo  = flag.Bool("cgo", false, "")
	race = flag.Bool("race", false, "")
	os_  = flag.String("os", "", "")
	arch = flag.String("arch", "", "")
)

func main() {
	flag.Usage = func() {
		fmt.Println(usage)
	}
	flag.Parse()

	switch target := flag.Arg(0); target {
	case "clean":
		reset(false)
	case "clear":
		reset(true)
	case "dist":
		fmt.Println("dist is not implemented yet")
	default: // build
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
		if *os_ != "" {
			os.Setenv("GOOS", *os_)
		}
		if *arch != "" {
			os.Setenv("GOARCH", *arch)
		}
		switch target {
		case "all":
			cmds, err := os.ReadDir("cmds")
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			for _, cmd := range cmds {
				if cmd.IsDir() {
					name := cmd.Name()
					build(name)
				}
			}
			fallthrough
		case "":
			build("")
		default:
			build(target)
		}
	}
}

func build(name string) {
	var cmd *exec.Cmd
	if name == "" {
		if *race {
			cmd = exec.Command("go", "build", "-race")
		} else {
			cmd = exec.Command("go", "build")
		}
	} else {
		if *race {
			cmd = exec.Command("go", "build", "-race", "github.com/hexinfra/gorox/cmds/"+name)
		} else {
			cmd = exec.Command("go", "build", "github.com/hexinfra/gorox/cmds/"+name)
		}
	}
	fmt.Printf("building %s...", name)
	if out, _ := cmd.CombinedOutput(); len(out) > 0 {
		fmt.Println(string(out))
	} else {
		fmt.Println("ok.")
	}
}

func reset(withVars bool) {
	pwd, err := os.Getwd()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	dirs := []string{
		"dist",
		"logs",
		"temp",
	}
	if withVars {
		dirs = append(dirs, "vars")
	}
	for _, dir := range dirs {
		dir = pwd + "/" + dir
		if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
			fmt.Println(err.Error())
			return
		}
	}

	names := []string{filepath.Base(pwd)}
	cmds, err := os.ReadDir("cmds")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	for _, cmd := range cmds {
		if !cmd.IsDir() {
			continue
		}
		names = append(names, cmd.Name())
	}
	exts := []string{"", ".exe", ".exe~"}
	for _, name := range names {
		for _, ext := range exts {
			file := pwd + "/" + name + ext
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				fmt.Println(err.Error())
				return
			}
		}
	}

	fmt.Println("done.")
}
