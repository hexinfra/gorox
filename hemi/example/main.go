package main

import (
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"
	"os"
	"path/filepath"
	"runtime"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	exePath, err := os.Executable()
	must(err)
	baseDir := filepath.Dir(exePath)
	if runtime.GOOS == "windows" {
		baseDir = filepath.ToSlash(baseDir)
	}

	// Begin
	RegisterHandlet("myHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(myHandlet)
		h.onCreate(name, stage, app)
		return h
	})

	SetBaseDir(baseDir)
	SetDataDir(baseDir + "/data")
	SetLogsDir(baseDir + "/logs")
	SetTempDir(baseDir + "/temp")

	stage, err := ApplyText(config)
	must(err)
	stage.Start(0)
	// End

	select {}
}

var config = `
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
	r.Link("/foo", h.foo)
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

func (h *myHandlet) GET_(req Request, resp Response) {
	resp.Send("hello, world!")
}
func (h *myHandlet) POST_login(req Request, resp Response) {
	resp.Send("what are you doing?")
}
func (h *myHandlet) foo(req Request, resp Response) {
	resp.Push("foo")
	resp.Push("bar")
}
