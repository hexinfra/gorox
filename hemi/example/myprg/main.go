package main

import (
	"github.com/hexinfra/gorox/hemi/procmgr"

	_ "myprg/apps"
	_ "myprg/exts"
	_ "myprg/svcs"
)

func main() {
	procmgr.Main(&procmgr.Opts{
		ProgramName:  "myprg",
		ProgramTitle: "MyProgram",
		DebugLevel:   0,
		CmdUIAddr:    "127.0.0.1:9527",
		WebUIAddr:    "127.0.0.1:9528",
	})
}
