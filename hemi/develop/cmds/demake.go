// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Demake builds cmds.

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

  demake [OPTIONS] [TARGET]

OPTIONS
-------

  -fmt              # run gofmt before building
  -cgo              # enable cgo
  -race             # enable race detection
  -os   <goos>      # GOOS
  -arch <goarch>    # GOARCH

  Options only take effect on building.

TARGET
------

  Specify cmd name as TARGET. If TARGET is empty, the default target is derox.
  Some special targets are:

  all      # build all cmds in the directory
  clean    # clean binaries, logs, and temp files
`

var (
	fmt_ = flag.Bool("fmt", false, "")
	cgo  = flag.Bool("cgo", false, "")
	race = flag.Bool("race", false, "")
	os_  = flag.String("os", "", "")
	arch = flag.String("arch", "", "")
)

func main() {
	_, err := os.Stat("hemi")
	if err == nil {
		fmt.Println("please use gomake.")
		return
	}

	flag.Usage = func() {
		fmt.Println(usage)
	}
	flag.Parse()

	switch target := flag.Arg(0); target {
	case "clean":
		clean()
	default: // build
		if *fmt_ {
			cmd := exec.Command("gofmt", "-w", "..")
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
		case "":
			build("derox", "cmds/derox")
		case "all":
			cmds, err := os.ReadDir("cmds")
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			for _, cmd := range cmds {
				if cmd.IsDir() {
					name := cmd.Name()
					build(name, "cmds/"+name)
				}
			}
		default:
			build(target, "cmds/"+target)
		}
	}
}

func clean() {
	pwd, err := os.Getwd()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	dirs := []string{
		"logs",
		"temp",
	}
	for _, dir := range dirs {
		dir = pwd + "/" + dir
		if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
			fmt.Println(err.Error())
			return
		}
	}
	cmds, err := os.ReadDir("cmds")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	for _, cmd := range cmds {
		if !cmd.IsDir() {
			continue
		}
		name := cmd.Name()
		for _, ext := range []string{"", ".exe", ".exe~"} {
			file := pwd + "/" + name + ext
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				fmt.Println(err.Error())
				return
			}
		}
	}
	fmt.Println("clean ok.")
}

func build(name string, path string) {
	var cmd *exec.Cmd
	if *race {
		cmd = exec.Command("go", "build", "-race", "github.com/hexinfra/gorox/hemi/develop/"+path)
	} else {
		cmd = exec.Command("go", "build", "github.com/hexinfra/gorox/hemi/develop/"+path)
	}
	fmt.Printf("building %s...", name)
	if out, _ := cmd.CombinedOutput(); len(out) > 0 {
		fmt.Println(string(out))
	} else {
		fmt.Println("ok.")
	}
}
