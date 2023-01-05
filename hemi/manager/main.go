// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Manager implements leader-worker process model and its control agent.

package manager

import (
	"flag"
	"fmt"
	"github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"os"
	"path/filepath"
	"strings"
)

var (
	progName string
	procArgs = append([]string{system.ExePath}, os.Args[1:]...)
)

var ( // flags
	debugLevel int
	targetAddr string
	adminAddr  string
	goopsAddr  = flag.String("goops", "", "")
	tryRun     = flag.Bool("try", false, "")
	singleMode = flag.Bool("single", false, "")
	daemonMode = flag.Bool("daemon", false, "")
	logFile    = flag.String("log", "", "")
	baseDir    = flag.String("base", "", "")
	logsDir    = flag.String("logs", "", "")
	tempDir    = flag.String("temp", "", "")
	varsDir    = flag.String("vars", "", "")
	config     = flag.String("config", "", "")
)

func Main(name string, usage string, level int, addr string) {
	if !system.Check() {
		crash("current platform (os+arch) is not supported.")
	}
	progName = name

	flag.Usage = func() {
		fmt.Printf(usage, hemi.Version)
	}
	flag.IntVar(&debugLevel, "debug", level, "")
	flag.StringVar(&targetAddr, "target", addr, "")
	flag.StringVar(&adminAddr, "admin", addr, "")
	action := "serve"
	if len(os.Args) > 1 && os.Args[1][0] != '-' {
		action = os.Args[1]
		flag.CommandLine.Parse(os.Args[2:])
	} else {
		flag.Parse()
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
		hemi.SetDebug(int32(debugLevel))
		serve()
	}
}

func serve() { // as single, leader, or worker
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

	setDir := func(pDir *string, name string, set func(string)) {
		if dir := *pDir; dir == "" {
			*pDir = *baseDir + "/" + name
		} else if !filepath.IsAbs(dir) {
			*pDir = *baseDir + "/" + dir
		}
		*pDir = filepath.ToSlash(*pDir)
		set(*pDir)
	}
	setDir(logsDir, "logs", hemi.SetLogsDir)
	setDir(tempDir, "temp", hemi.SetTempDir)
	setDir(varsDir, "vars", hemi.SetVarsDir)

	if *tryRun { // for testing config file
		if _, err := hemi.ApplyFile(getConfig()); err != nil {
			fmt.Println(err.Error())
		} else {
			fmt.Println("PASS")
		}
	} else if *singleMode { // run as single foreground process. for single mode
		if stage, err := hemi.ApplyFile(getConfig()); err == nil {
			stage.Start(0)
			select {}
		} else {
			fmt.Println(err.Error())
		}
	} else if token, ok := os.LookupEnv("_DAEMON_"); ok { // run leader process as daemon
		if token == "leader" {
			system.DaemonInit()
			leaderMain()
		} else {
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

func getConfig() (base string, file string) {
	baseDir, config := *baseDir, *config
	if strings.HasPrefix(config, "http://") || strings.HasPrefix(config, "https://") {
		panic("currently not supported!")
	} else {
		if config == "" {
			base = baseDir
			file = "conf/" + progName + ".conf"
		} else if filepath.IsAbs(config) { // /path/to/file.conf
			base = filepath.Dir(config)
			file = filepath.Base(config)
		} else { // path/to/file.conf
			base = baseDir
			file = config
		}
		base += "/"
	}
	return
}

const ( // exit codes
	codeStop  = 10
	codeCrash = 11
)

func stop() {
	os.Exit(codeStop)
}
func crash(s string) {
	fmt.Fprintln(os.Stderr, s)
	os.Exit(codeCrash)
}
