// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Manager implements leader-worker process model and its control agent.

// Some terms:
//   admDoor - Used by leader process, for receiving msgConns from control agent.
//   msgConn - control agent ----> leader admin
//   msgChan - leaderMain() <---> keepWorkers()
//   dieChan - keepWorkers() <---> worker
//   msgPipe - leader process <---> worker process

package manager

import (
	"flag"
	"fmt"
	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const ( // proc modes
	ProcModeGeneral = 0
	ProcModeAlone   = 1
	ProcModeDevelop = 2
)

var program string

var ( // flags
	debugMode  = flag.Bool("debug", false, "")
	targetAddr string
	adminAddr  string
	develMode  = flag.Bool("devel", false, "")
	tryRun     = flag.Bool("try", false, "")
	baseDir    = flag.String("base", "", "")
	dataDir    = flag.String("data", "", "")
	logsDir    = flag.String("logs", "", "")
	tempDir    = flag.String("temp", "", "")
	config     = flag.String("config", "", "")
	logFile    = flag.String("log", "", "")
	userName   = flag.String("user", "nobody", "")
	multiple   = flag.Int("multi", 0, "")
	pinCPU     = flag.Bool("pin", false, "")
	daemonMode = flag.Bool("daemon", false, "")
)

func Main(name string, usage string, procMode int, addr string) {
	if !system.Check() {
		crash("current platform (os+arch) is not supported.")
	}
	program = name

	flag.Usage = func() {
		fmt.Printf(usage, hemi.Version)
	}
	flag.StringVar(&targetAddr, "target", addr, "")
	flag.StringVar(&adminAddr, "admin", addr, "")
	action := "serve"
	if len(os.Args) > 1 && os.Args[1][0] != '-' {
		action = os.Args[1]
		flag.CommandLine.Parse(os.Args[2:])
	} else {
		flag.Parse()
	}

	if procMode == ProcModeAlone {
		*multiple = 0
	} else {
		ncpu := runtime.NumCPU()
		if *multiple > ncpu {
			*multiple = ncpu
		}
		if procMode == ProcModeDevelop {
			*debugMode = true
			*develMode = true
		}
	}

	if action == "help" {
		fmt.Printf(usage, hemi.Version)
	} else if action == "version" {
		fmt.Println(hemi.Version)
	} else if action == "advise" {
		system.Advise()
	} else if action != "serve" { // as control agent
		agentMain(action)
	} else { // run as server
		serve()
	}
}

func serve() { // as leader or worker
	if *debugMode {
		hemi.SetDebug(true)
	}
	if *develMode {
		hemi.SetDevel(true)
	}

	// baseDir
	if *baseDir == "" {
		*baseDir = system.ExeDir
	} else { // baseDir is specified.
		dir, err := filepath.Abs(*baseDir)
		if err != nil {
			crash(err.Error())
		}
		*baseDir = dir
	}
	*baseDir = filepath.ToSlash(*baseDir)
	hemi.SetBaseDir(*baseDir)

	// dataDir
	if dir := *dataDir; dir == "" {
		*dataDir = *baseDir + "/data"
	} else if !filepath.IsAbs(dir) {
		*dataDir = *baseDir + "/" + dir
	}
	*dataDir = filepath.ToSlash(*dataDir)
	hemi.SetDataDir(*dataDir)

	// logsDir
	if dir := *logsDir; dir == "" {
		*logsDir = *baseDir + "/logs"
	} else if !filepath.IsAbs(dir) {
		*logsDir = *baseDir + "/" + dir
	}
	*logsDir = filepath.ToSlash(*logsDir)
	hemi.SetLogsDir(*logsDir)

	// tempDir
	if dir := *tempDir; dir == "" {
		*tempDir = *baseDir + "/temp"
	} else if !filepath.IsAbs(dir) {
		*tempDir = *baseDir + "/" + dir
	}
	*tempDir = filepath.ToSlash(*tempDir)
	hemi.SetTempDir(*tempDir)

	if *develMode { // run as foreground worker. for developer mode
		develMain()
	} else if *tryRun { // for testing config file
		if _, err := hemi.ApplyFile(getConfig()); err != nil {
			fmt.Println(err.Error())
		} else {
			fmt.Println("PASS")
		}
	} else if token, ok := os.LookupEnv("_DAEMON_"); ok { // run (leader or worker) process as daemon
		if token == "leader" { // run leader process as daemon
			system.DaemonInit()
			leaderMain()
		} else { // run worker process as daemon
			// Don't system.DaemonInit() here, as it causes bugs on Windows, under which no stderr outputs are shown
			workerMain(token)
		}
	} else if *daemonMode { // start the leader daemon and exit
		devNull, err := os.Open(os.DevNull)
		if err != nil {
			crash(err.Error())
		}
		if leader, err := os.StartProcess(system.ExePath, procArgs, &os.ProcAttr{
			Env:   []string{"_DAEMON_=leader", "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")},
			Files: []*os.File{devNull, devNull, devNull},
			Sys:   system.DaemonSysAttr(),
		}); err == nil {
			leader.Release()
			devNull.Close()
		} else {
			crash(err.Error())
		}
	} else { // run as foreground leader. default case
		leaderMain()
	}
}

var procArgs = append([]string{system.ExePath}, os.Args[1:]...)

const ( // exit codes
	codeCrash = 10
	codeStop  = 11
)

func crash(s string) {
	fmt.Fprintln(os.Stderr, s)
	os.Exit(codeCrash)
}
func stop() {
	os.Exit(codeStop)
}

func getConfig() (base string, file string) {
	baseDir, config := *baseDir, *config
	if strings.HasPrefix(config, "http://") || strings.HasPrefix(config, "https://") {
		panic("currently not supported!")
	} else {
		if config == "" {
			base = baseDir
			file = "conf/" + program + ".conf"
		} else if filepath.IsAbs(config) { // /path/to/file.conf
			base = filepath.Dir(config)
			file = config
		} else { // path/to/file.conf
			base = baseDir
			file = baseDir + "/" + config
		}
		base += "/"
	}
	return
}
