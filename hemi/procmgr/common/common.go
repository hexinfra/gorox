// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Common elements.

package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hexinfra/gorox/hemi/common/system"
)

var (
	DebugLevel int
	TargetAddr string
	CmdUIAddr  string
	WebUIAddr  string
	MyroxAddr  string
	ConfigFile string
	SingleMode bool
	DaemonMode bool
	TopDir     string
	LogDir     string
	TmpDir     string
	VarDir     string
	Stdout     string
	Stderr     string
)

func GetConfig() (configBase string, configFile string) {
	if strings.HasPrefix(ConfigFile, "http://") || strings.HasPrefix(ConfigFile, "https://") {
		// base: scheme://host:port/prefix
		// file: /program.conf
		panic("currently not supported!")
	} else {
		if ConfigFile == "" {
			configBase = TopDir
			configFile = "conf/" + Program + ".conf"
		} else if filepath.IsAbs(ConfigFile) { // /path/to/file.conf
			configBase = filepath.Dir(ConfigFile)
			configFile = filepath.Base(ConfigFile)
		} else { // path/to/file.conf
			configBase = TopDir
			configFile = ConfigFile
		}
		configBase += "/"
	}
	return
}

var (
	Program string                                             // gorox, myrox, ...
	ExeArgs = append([]string{system.ExePath}, os.Args[1:]...) // /path/to/exe arg1 arg2 ...
)

const (
	CodeStop  = 10
	CodeCrash = 11
)

func Stop() {
	os.Exit(CodeStop)
}

func Crash(s string) {
	fmt.Fprintln(os.Stderr, s)
	os.Exit(CodeCrash)
}

const ( // calls
	ComdPids   = iota // report pids of leader and worker
	ComdLeader        // report leader info
	ComdWorker        // report worker info
	ComdReload        // reload worker config
)

const ( // tells
	ComdStop      = iota // exit server immediately
	ComdQuit             // exit server gracefully
	ComdRecmd            // reopen cmdui interface
	ComdReweb            // reopen webui interface
	ComdRework           // restart worker process
	ComdCPU              // profile cpu
	ComdHeap             // profile heap
	ComdThread           // profile thread
	ComdGoroutine        // profile goroutine
	ComdBlock            // profile block
	ComdGC               // run runtime.GC()
)
