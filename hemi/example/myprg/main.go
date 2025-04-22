package main

import (
	"github.com/hexinfra/gorox/hemi/process"

	_ "myprg/apps" // all web applications
	_ "myprg/exts" // all hemi extensions
	_ "myprg/svcs" // all rpc services
)

func main() {
	process.Main(&process.Opts{
		ProgramName:  "myprg",
		ProgramTitle: "MyProgram",
		DebugLevel:   0,
		CmdUIAddr:    "127.0.0.1:9527",
		WebUIAddr:    "127.0.0.1:9528",
	})
}
