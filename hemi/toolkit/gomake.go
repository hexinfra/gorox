// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
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

  Specify cmd name as TARGET. If empty, the default action is: go build .
  Some special TARGETs are:

  all      # build all cmds in the directory
  clean    # clean bins, logs, and temp files
  clear    # clear bins, logs, temp, and vars files
  dist     # make distribution
`

var (
	fmt_ = flag.Bool("fmt", false, "")
	cgo  = flag.Bool("cgo", false, "")
	race = flag.Bool("race", false, "")
	os_  = flag.String("os", "", "")
	arch = flag.String("arch", "", "")
)

var (
	workDir string // current working directory
	dirName string // filepath.Base(workDir)
)

func main() {
	pwd, err := os.Getwd()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	workDir = pwd
	name := filepath.Base(workDir)
	if name == "." || name == string(filepath.Separator) {
		fmt.Println("bad working directory!")
		return
	}
	dirName = name

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
			rootDir := workDir
			for {
				if _, err := os.Stat(rootDir + "/go.mod"); err == nil {
					break
				}
				rootDir = filepath.Dir(rootDir)
				if rootDir == "." || rootDir == string(filepath.Separator) {
					fmt.Println("no go.mod found!")
					return
				}
			}
			cmd := exec.Command("gofmt", "-w", rootDir)
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
		fmt.Printf("building %s...", dirName)
	} else {
		path := "cmds/" + name
		if *race {
			cmd = exec.Command("go", "build", "-race", "github.com/hexinfra/gorox/"+path)
		} else {
			cmd = exec.Command("go", "build", "github.com/hexinfra/gorox/"+path)
		}
		fmt.Printf("building %s...", path)
	}
	if out, _ := cmd.CombinedOutput(); len(out) > 0 {
		fmt.Println(string(out))
	} else {
		fmt.Println("ok.")
	}
}

func reset(withVars bool) {
	names := []string{
		"logs",
		"temp",
	}
	if withVars {
		names = append(names, "vars")
	}
	for _, name := range names {
		dir := workDir + "/" + name
		if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
			fmt.Println(err.Error())
			return
		}
	}

	names = []string{dirName}
	if cmds, err := os.ReadDir("cmds"); err == nil {
		for _, cmd := range cmds {
			if cmd.IsDir() {
				names = append(names, cmd.Name())
			}
		}
	}
	exts := []string{"", ".exe", ".exe~"}
	for _, name := range names {
		for _, ext := range exts {
			file := workDir + "/" + name + ext
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				fmt.Println(err.Error())
				return
			}
		}
	}

	fmt.Println("done.")
}
