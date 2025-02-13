package myapp

import (
	"github.com/hexinfra/gorox/hemi/builtin/mappers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("myHandlet", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(myHandlet)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

type myHandlet struct {
	Handlet_
}

func (h *myHandlet) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)

	m := simple.New()
	m.Map("/foo", h.handleFoo)
	h.UseMapper(h, m)
}
func (h *myHandlet) OnShutdown() { h.Webapp().DecHandlet() }

func (h *myHandlet) OnConfigure() {}
func (h *myHandlet) OnPrepare()   {}

func (h *myHandlet) Handle(req ServerRequest, resp ServerResponse) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *myHandlet) notFound(req ServerRequest, resp ServerResponse) {
	resp.Send("404 handle not found!")
}

func (h *myHandlet) handleFoo(req ServerRequest, resp ServerResponse) { // METHOD /foo
	resp.Echo(req.H("user-agent"))
}

func (h *myHandlet) GET_(req ServerRequest, resp ServerResponse) { // GET /
	resp.Echo("hello, world! ")
	resp.Echo("this is myapp.")
}
func (h *myHandlet) POST_user_login(req ServerRequest, resp ServerResponse) { // POST /user/login
	resp.Send("what are you doing?")
}
