package main

import (
	"github.com/hexinfra/gorox/hemi/procman"

	_ "myprg/apps"
	_ "myprg/exts"
	_ "myprg/svcs"
)

func main() {
	procman.Main(&procman.Opts{
		ProgramName:  "myprg",
		ProgramTitle: "MyProgram",
		DebugLevel:   0,
		CmdUIAddr:    "127.0.0.1:9527",
		WebUIAddr:    "127.0.0.1:9528",
	})
}
