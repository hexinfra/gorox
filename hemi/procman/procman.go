// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Procman package implements a leader-worker process model and its control client.

package procman

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/common/system"
	"github.com/hexinfra/gorox/hemi/procman/client"
	"github.com/hexinfra/gorox/hemi/procman/common"
	"github.com/hexinfra/gorox/hemi/procman/leader"
	"github.com/hexinfra/gorox/hemi/procman/worker"
)

func Main(program string, usage string, level int, addr string) {
	if !system.Check() {
		common.Crash("current platform (os + arch) is not supported.")
	}

	common.Program = program

	flag.Usage = func() { fmt.Printf(usage, hemi.Version) }
	flag.IntVar(&common.DebugLevel, "debug", level, "")
	flag.StringVar(&common.TargetAddr, "target", addr, "")
	flag.StringVar(&common.AdminAddr, "admin", addr, "")
	flag.StringVar(&common.MyroxAddr, "myrox", "", "")
	flag.StringVar(&common.Config, "conf", "", "")
	flag.BoolVar(&common.SingleMode, "single", false, "")
	flag.BoolVar(&common.DaemonMode, "daemon", false, "")
	flag.StringVar(&common.LogFile, "log", "", "")
	flag.StringVar(&common.BaseDir, "base", "", "")
	flag.StringVar(&common.LogsDir, "logs", "", "")
	flag.StringVar(&common.TempDir, "temp", "", "")
	flag.StringVar(&common.VarsDir, "vars", "", "")
	action := "serve"
	if len(os.Args) > 1 && os.Args[1][0] != '-' {
		action = os.Args[1]
		flag.CommandLine.Parse(os.Args[2:])
	} else {
		flag.Parse()
	}

	switch action {
	case "help":
		fmt.Printf(usage, hemi.Version)
	case "version":
		fmt.Println(hemi.Version)
	case "advise":
		system.Advise()
	case "serve", "check":
		hemi.SetDebug(int32(common.DebugLevel))
		if common.BaseDir == "" {
			common.BaseDir = system.ExeDir
		} else { // baseDir is specified.
			dir, err := filepath.Abs(common.BaseDir)
			if err != nil {
				common.Crash(err.Error())
			}
			common.BaseDir = dir
		}
		common.BaseDir = filepath.ToSlash(common.BaseDir)
		hemi.SetBaseDir(common.BaseDir)
		setDir := func(pDir *string, name string, set func(string)) {
			if dir := *pDir; dir == "" {
				*pDir = common.BaseDir + "/" + name
			} else if !filepath.IsAbs(dir) {
				*pDir = common.BaseDir + "/" + dir
			}
			*pDir = filepath.ToSlash(*pDir)
			set(*pDir)
		}
		setDir(&common.LogsDir, "logs", hemi.SetLogsDir)
		setDir(&common.TempDir, "temp", hemi.SetTempDir)
		setDir(&common.VarsDir, "vars", hemi.SetVarsDir)

		if action == "check" { // dry run
			if _, err := hemi.ApplyFile(common.GetConfig()); err != nil {
				fmt.Println(err.Error())
			} else {
				fmt.Println("PASS")
			}
			return
		}

		// Serve.
		if common.SingleMode { // run as single foreground process. for single mode
			if stage, err := hemi.ApplyFile(common.GetConfig()); err == nil {
				stage.Start(0)
				select {} // waiting forever
			} else {
				fmt.Println(err.Error())
			}
		} else if token, ok := os.LookupEnv("_DAEMON_"); ok { // run leader process as daemon
			if token == "leader" { // leader daemon
				system.DaemonInit()
				leader.Main()
			} else { // worker daemon
				worker.Main(token)
			}
		} else if common.DaemonMode { // start the leader daemon and exit
			devNull, err := os.Open(os.DevNull)
			if err != nil {
				common.Crash(err.Error())
			}
			if process, err := os.StartProcess(system.ExePath, common.ExeArgs, &os.ProcAttr{
				Env:   []string{"_DAEMON_=leader", "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")},
				Files: []*os.File{devNull, devNull, devNull},
				Sys:   system.DaemonSysAttr(),
			}); err == nil { // leader process started
				process.Release()
				devNull.Close()
			} else {
				common.Crash(err.Error())
			}
		} else { // run as foreground leader. default case
			leader.Main()
		}
	default: // as control client
		client.Main(action)
	}
}
