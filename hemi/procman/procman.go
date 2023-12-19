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

const usage = `
%s (%s)
================================================================================

  %s [ACTION] [OPTIONS]

ACTION
------

  serve      # start as server
  check      # dry run to check config
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
  reload     # call worker to reload config
  cpu        # tell worker to perform cpu profiling
  heap       # tell worker to perform heap profiling
  thread     # tell worker to perform thread profiling
  goroutine  # tell worker to perform goroutine profiling
  block      # tell worker to perform block profiling

  Only one action is allowed at a time.
  If ACTION is not specified, the default action is "serve".

OPTIONS
-------

  -debug  <level>   # debug level (default: %d. min: 0, max: 3)
  -target <addr>    # leader address to tell or call (default: %s)
  -cmdui  <addr>    # listen address of leader cmdui (default: %s)
  -webui  <addr>    # listen address of leader webui (default: %s)
  -myrox  <addr>    # myrox to use. "-cmdui" and "-webui" will be ignored if set
  -config <config>  # path or url to worker config file
  -single           # run server in single mode. only a process is started
  -daemon           # run server as daemon (default: false)
  -base   <path>    # base directory of the program
  -logs   <path>    # logs directory to use
  -tmps   <path>    # tmps directory to use
  -vars   <path>    # vars directory to use
  -stdout <path>    # daemon's stdout file (default: %s.out in logs dir)
  -stderr <path>    # daemon's stderr file (default: %s.err in logs dir)

  "-debug" applies to all actions.
  "-target" applies to telling and calling actions only.
  "-cmdui" applies to "serve" and "recmd".
  "-webui" applies to "serve" and "reweb".
  Other options apply to "serve" only.

`

type Args struct {
	Title      string
	Program    string
	DebugLevel int
	CmdUIAddr  string
	WebUIAddr  string
	Usage      string
}

func Main(args *Args) {
	if !system.Check() {
		common.Crash("current platform (os + arch) is not supported.")
	}

	common.Program = args.Program

	flag.Usage = func() {
		if args.Usage == "" {
			fmt.Printf(usage, args.Title, hemi.Version, args.Program, args.DebugLevel, args.CmdUIAddr, args.CmdUIAddr, args.WebUIAddr, args.Program, args.Program)
		} else {
			fmt.Println(args.Usage)
		}
	}
	flag.IntVar(&common.DebugLevel, "debug", args.DebugLevel, "")
	flag.StringVar(&common.TargetAddr, "target", args.CmdUIAddr, "")
	flag.StringVar(&common.CmdUIAddr, "cmdui", args.CmdUIAddr, "")
	flag.StringVar(&common.WebUIAddr, "webui", args.WebUIAddr, "")
	flag.StringVar(&common.MyroxAddr, "myrox", "", "")
	flag.StringVar(&common.ConfigFile, "config", "", "")
	flag.BoolVar(&common.SingleMode, "single", false, "")
	flag.BoolVar(&common.DaemonMode, "daemon", false, "")
	flag.StringVar(&common.BaseDir, "base", "", "")
	flag.StringVar(&common.LogsDir, "logs", "", "")
	flag.StringVar(&common.TmpsDir, "tmps", "", "")
	flag.StringVar(&common.VarsDir, "vars", "", "")
	flag.StringVar(&common.OutFile, "stdout", "", "")
	flag.StringVar(&common.ErrFile, "stderr", "", "")
	action := "serve"
	if len(os.Args) > 1 && os.Args[1][0] != '-' {
		action = os.Args[1]
		flag.CommandLine.Parse(os.Args[2:])
	} else {
		flag.Parse()
	}

	switch action {
	case "help":
		flag.Usage()
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
		setDir(&common.TmpsDir, "tmps", hemi.SetTmpsDir)
		setDir(&common.VarsDir, "vars", hemi.SetVarsDir)

		if action == "check" { // dry run
			if _, err := hemi.BootFile(common.GetConfig()); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
			} else {
				fmt.Println("PASS")
			}
			return
		}

		// Now serve.
		if common.SingleMode { // run as single foreground process. for single mode
			if stage, err := hemi.BootFile(common.GetConfig()); err == nil {
				stage.Start(0)
				select {} // waiting forever
			} else {
				fmt.Fprintln(os.Stderr, err.Error())
			}
		} else if token, ok := os.LookupEnv("_DAEMON_"); ok { // run leader process as daemon
			if token == "leader" { // leader daemon
				system.DaemonInit()
				leader.Main()
			} else { // worker daemon
				worker.Main(token)
			}
		} else if common.DaemonMode { // start leader daemon and exit
			newFile := func(file string, ext string, osFile *os.File) *os.File {
				if file == "" {
					file = common.LogsDir + "/" + common.Program + ext
				} else if !filepath.IsAbs(file) {
					file = common.BaseDir + "/" + file
				}
				if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
					common.Crash(err.Error())
				}
				if !common.DaemonMode {
					osFile.Close()
				}
				osFile, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0700)
				if err != nil {
					common.Crash(err.Error())
				}
				return osFile
			}
			stdout := newFile(common.OutFile, ".out", os.Stdout)
			stderr := newFile(common.ErrFile, ".err", os.Stderr)
			devNull, err := os.Open(os.DevNull)
			if err != nil {
				common.Crash(err.Error())
			}
			if process, err := os.StartProcess(system.ExePath, common.ExeArgs, &os.ProcAttr{
				Env:   []string{"_DAEMON_=leader", "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")},
				Files: []*os.File{devNull, stdout, stderr},
				Sys:   system.DaemonSysAttr(),
			}); err == nil { // leader process started
				process.Release()
				devNull.Close()
				stdout.Close()
				stderr.Close()
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
