// This is an example showing how to embed the Hemi engine into your application.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

import (
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"
)

func main() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	baseDir := filepath.Dir(exePath)
	if runtime.GOOS == "windows" {
		baseDir = filepath.ToSlash(baseDir)
	}

	var myConfig = `
	stage {
	    app "example" {
		.hostnames = ("*")
		.webRoot   = %baseDir + "/web"
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
	if err := startHemi(baseDir, myConfig); err != nil {
		fmt.Println(err.Error())
		return
	}

	select {} // do your other things here.
}

func startHemi(baseDir string, configText string) error {
	RegisterHandlet("myHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(myHandlet)
		h.onCreate(name, stage, app)
		return h
	})

	SetBaseDir(baseDir)
	SetDataDir(baseDir + "/data")
	SetLogsDir(baseDir + "/logs")
	SetTempDir(baseDir + "/temp")

	stage, err := ApplyText(configText)
	if err != nil {
		return err
	}
	stage.Start(0)
	return nil
}

type myHandlet struct {
	Handlet_
	stage *Stage
	app   *App
}

func (h *myHandlet) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
	r := simple.New()
	r.Link("/foo", h.handleFoo)
	h.SetRouter(h, r)
}
func (h *myHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *myHandlet) OnConfigure() {}
func (h *myHandlet) OnPrepare()   {}

func (h *myHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *myHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}

func (h *myHandlet) GET_(req Request, resp Response) { // GET /
	resp.Send("hello, world!")
}
func (h *myHandlet) POST_login(req Request, resp Response) { // POST /login
	resp.Send("what are you doing?")
}
func (h *myHandlet) handleFoo(req Request, resp Response) { // METHOD /foo
	resp.Echo(req.H("user-agent"))
}
