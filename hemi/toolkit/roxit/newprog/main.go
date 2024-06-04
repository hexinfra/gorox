package main

import (
	"github.com/hexinfra/gorox/hemi/procmgr"
)

func main() {
	procmgr.Main(&procmgr.Args{
		Program:    "newprog",
		Title:      "NewProg",
		DebugLevel: 0,
		CmdUIAddr:  "127.0.0.1:9527",
		WebUIAddr:  "127.0.0.1:9528",
	})
}
