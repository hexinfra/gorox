// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Procman package implements a leader-worker process model and its control client.

package procman

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/library/system"

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

  serve      # start as server. the default action
  help       # show this message
  version    # show version info
  check      # dry run to check the config
  advise     # show how to optimize current platform
  pids       # call server to report pids of leader and worker
  stop       # tell server to exit immediately
  quit       # tell server to exit gracefully
  leader     # call leader to report its info
  recmd      # tell leader to reopen its cmdui interface
  reweb      # tell leader to reopen its webui interface
  rework     # tell leader to restart worker gracefully
  worker     # call worker to report its info
  reconf     # call worker to reload config
  cpu        # tell worker to perform cpu profiling
  heap       # tell worker to perform heap profiling
  thread     # tell worker to perform thread profiling
  goroutine  # tell worker to perform goroutine profiling
  block      # tell worker to perform block profiling

  Only one action is allowed at a time.
  If ACTION is not specified, the default action is "serve".

OPTIONS
-------

  -debug   <level>  # debug level (default: %d. min: 0, max: 3). 0 means disable
  -target  <addr>   # leader address to tell or call (default: %s)
  -cmdui   <addr>   # listen address of leader cmdui (default: %s)
  -webui   <addr>   # listen address of leader webui (default: %s)
  -rockman <addr>   # rockman to use. -cmdui and -webui will be ignored if set
  -config  <config> # path to worker config file (default: conf/%s.conf)
  -single           # run server in single mode. only a process is started
  -daemon           # run server as daemon (default: false)
  -topDir  <path>   # top directory of the program files
  -logDir  <path>   # log directory to use
  -tmpDir  <path>   # tmp directory to use
  -varDir  <path>   # var directory to use
  -stdout  <path>   # daemon's stdout file (default: %s.out in log directory)
  -stderr  <path>   # daemon's stderr file (default: %s.err in log directory)

  "-debug" applies to all actions.
  "-target" applies to telling and calling actions only.
  "-cmdui" applies to "serve" and "recmd".
  "-webui" applies to "serve" and "reweb".
  Other options apply to "serve" only.

`

// Opts is the options passed to Main() to control its behavior.
type Opts struct {
	ProgramName  string
	ProgramTitle string
	ProgramUsage string
	DebugLevel   int
	CmdUIAddr    string
	WebUIAddr    string
}

// Main is the main() for client process, leader process, and worker process.
func Main(opts *Opts) {
	if !system.Check() {
		common.Crash("current platform (os + arch) is not supported.")
	}

	common.ProgramName = opts.ProgramName

	flag.Usage = func() {
		if opts.ProgramUsage == "" {
			fmt.Printf(usage, opts.ProgramTitle, hemi.Version, opts.ProgramName, opts.DebugLevel, opts.CmdUIAddr, opts.CmdUIAddr, opts.WebUIAddr, opts.ProgramName, opts.ProgramName, opts.ProgramName)
		} else {
			fmt.Println(opts.ProgramUsage)
		}
	}
	flag.IntVar(&common.DebugLevel, "debug", opts.DebugLevel, "")
	flag.StringVar(&common.TargetAddr, "target", opts.CmdUIAddr, "")
	flag.StringVar(&common.CmdUIAddr, "cmdui", opts.CmdUIAddr, "")
	flag.StringVar(&common.WebUIAddr, "webui", opts.WebUIAddr, "")
	flag.StringVar(&common.RockmanAddr, "rockman", "", "")
	flag.StringVar(&common.ConfigFile, "config", "", "")
	flag.BoolVar(&common.SingleMode, "single", false, "")
	flag.BoolVar(&common.DaemonMode, "daemon", false, "")
	flag.StringVar(&common.TopDir, "topDir", "", "")
	flag.StringVar(&common.LogDir, "logDir", "", "")
	flag.StringVar(&common.TmpDir, "tmpDir", "", "")
	flag.StringVar(&common.VarDir, "varDir", "", "")
	flag.StringVar(&common.Stdout, "stdout", "", "")
	flag.StringVar(&common.Stderr, "stderr", "", "")
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
		if common.TopDir == "" { // topDir is not specified
			common.TopDir = system.ExeDir
		} else { // topDir is specified
			dir, err := filepath.Abs(common.TopDir)
			if err != nil {
				common.Crash(err.Error())
			}
			common.TopDir = dir
		}
		common.TopDir = filepath.ToSlash(common.TopDir)
		hemi.SetTopDir(common.TopDir)

		setDir := func(pDir *string, name string, set func(string)) {
			if dir := *pDir; dir == "" {
				*pDir = common.TopDir + "/data/" + name
			} else if !filepath.IsAbs(dir) {
				*pDir = common.TopDir + "/" + dir
			}
			*pDir = filepath.ToSlash(*pDir)
			set(*pDir)
		}
		setDir(&common.LogDir, "log", hemi.SetLogDir)
		setDir(&common.TmpDir, "tmp", hemi.SetTmpDir)
		setDir(&common.VarDir, "var", hemi.SetVarDir)

		hemi.SetDebugLevel(int32(common.DebugLevel))

		if action == "check" { // dry run
			if _, err := hemi.StageFromFile(common.GetConfig()); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
			} else {
				fmt.Println("PASS")
			}
		} else if common.SingleMode { // run as single foreground process. for single mode
			if stage, err := hemi.StageFromFile(common.GetConfig()); err == nil {
				stage.Start(0)
				select {} // waiting forever
			} else {
				fmt.Fprintln(os.Stderr, err.Error())
			}
		} else if token, ok := os.LookupEnv("_GOROX_DAEMON_"); ok { // run leader or worker as a daemon
			if token == "leader" { // leader daemon
				system.DaemonInit()
				leader.Main()
			} else { // worker daemon
				worker.Main(token)
			}
		} else if common.DaemonMode { // start leader daemon and exit
			newFile := func(file string, ext string, osFile *os.File) *os.File {
				if file == "" {
					file = common.LogDir + "/" + common.ProgramName + ext
				} else if !filepath.IsAbs(file) {
					file = common.TopDir + "/" + file
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
			stdout := newFile(common.Stdout, ".out", os.Stdout)
			stderr := newFile(common.Stderr, ".err", os.Stderr)
			devNull, err := os.Open(os.DevNull)
			if err != nil {
				common.Crash(err.Error())
			}
			if process, err := os.StartProcess(system.ExePath, common.ProgramArgs, &os.ProcAttr{
				Env:   []string{"_GOROX_DAEMON_=leader", "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")},
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
