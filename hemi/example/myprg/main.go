package main

import (
	"github.com/hexinfra/gorox/hemi/control"

	_ "myprg/apps" // all web applications
	_ "myprg/exts" // all hemi extensions
	_ "myprg/svcs" // all rpc services
)

func main() {
	control.Start(&control.Options{
		ProgramName:  "myprg",
		ProgramTitle: "MyProgram",
		DebugLevel:   0,
		CmdUIAddr:    "127.0.0.1:9527",
		WebUIAddr:    "127.0.0.1:9528",
	})
}
