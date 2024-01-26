package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func main() {
	embedHemi()
	select {} // do your other things here.
}

func embedHemi() {
	myConfig := `
	stage {
	    webapp "example" {
		.hostnames = ("*")
		.webRoot   = %baseDir + "/root"
		rule $path == "/favicon.ico" {
		    favicon {}
		}
		rule $path -f {
		    static {
			.autoIndex = true
		    }
		}
		rule {
		    myHandlet {}
		}
	    }
	    httpxServer "main" {
		.forApps = ("example")
		.address = ":3080"
	    }
	}
	`
	RegisterHandlet("myHandlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(myHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
	exePath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	baseDir := filepath.Dir(exePath)
	if runtime.GOOS == "windows" {
		baseDir = filepath.ToSlash(baseDir)
	}
	SetBaseDir(baseDir)
	SetLogsDir(baseDir + "/logs")
	SetTmpsDir(baseDir + "/tmps")
	SetVarsDir(baseDir + "/vars")
	stage, err := BootText(myConfig)
	if err != nil {
		panic(err)
	}
	stage.Start(0)
}

type myHandlet struct {
	Handlet_
	stage  *Stage
	webapp *Webapp
}

func (h *myHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp

	r := simple.New()

	r.Map("/foo", h.handleFoo)

	h.UseRouter(h, r)
}
func (h *myHandlet) OnShutdown() {
	h.webapp.SubDone()
}

func (h *myHandlet) OnConfigure() {}
func (h *myHandlet) OnPrepare()   {}

func (h *myHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *myHandlet) notFound(req Request, resp Response) {
	resp.Send("404 handle not found!")
}

func (h *myHandlet) handleFoo(req Request, resp Response) { // METHOD /foo
	if req.IsGET() {
		resp.Send("you are using GET method\n")
	} else {
		resp.Send("you are not using GET method\n")
	}
}

func (h *myHandlet) GET_(req Request, resp Response) { // GET /
	resp.Echo("hello, world! ")
	resp.Echo("this is an example application.")
}
func (h *myHandlet) POST_user_login(req Request, resp Response) { // POST /user/login
	resp.Send("what are you doing?")
}
