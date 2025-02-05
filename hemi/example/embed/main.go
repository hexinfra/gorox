package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hexinfra/gorox/hemi/builtin/mappers/simple"

	. "github.com/hexinfra/gorox/hemi"

	_ "github.com/hexinfra/gorox/hemi/builtin"
)

func main() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	topDir := filepath.Dir(exePath)
	if runtime.GOOS == "windows" {
		topDir = filepath.ToSlash(topDir)
	}

	if err := startHemi(topDir); err != nil {
		fmt.Println(err.Error())
		return
	}

	select {} // do your other things here.
}

func startHemi(topDir string) error {
	RegisterHandlet("myHandlet", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(myHandlet)
		h.onCreate(compName, stage, webapp)
		return h
	})
	SetTopDir(topDir)
	SetLogDir(topDir+"/log")
	SetTmpDir(topDir+"/tmp")
	SetVarDir(topDir+"/var")
	var configText = `
stage {
    webapp "example" {
        .hostnames = ("*")
        .webRoot   = %topDir + "/web"
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
        .webapps = ("example")
        .address = ":3080"
    }
}
`
	stage, err := StageFromText(configText)
	if err != nil {
		return err
	}
	stage.Start(0)
	return nil
}

// myHandlet
type myHandlet struct {
	Handlet_
}

func (h *myHandlet) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)

	m := simple.New()
	m.Map("/foo", h.handleFoo)
	h.UseMapper(h, m)
}
func (h *myHandlet) OnShutdown() {
	h.Webapp().DecSub()
}

func (h *myHandlet) OnConfigure() {}
func (h *myHandlet) OnPrepare()   {}

func (h *myHandlet) Handle(req ServerRequest, resp ServerResponse) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *myHandlet) notFound(req ServerRequest, resp ServerResponse) {
	resp.Send("handle not found!")
}

func (h *myHandlet) handleFoo(req ServerRequest, resp ServerResponse) { // METHOD /foo
	resp.Echo(req.H("user-agent"))
}

func (h *myHandlet) GET_(req ServerRequest, resp ServerResponse) { // GET /
	resp.Echo("hello, world! ")
	resp.Echo("this is embed.")
}
func (h *myHandlet) POST_user_login(req ServerRequest, resp ServerResponse) { // POST /user/login
	resp.Send("what are you doing?")
}
