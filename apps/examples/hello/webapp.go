package hello

import (
	"github.com/hexinfra/gorox/hemi/builtin/mappers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterWebappInit("hello", func(webapp *Webapp) error {
		return nil
	})
}

func init() {
	RegisterHandlet("helloHandlet", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(helloHandlet)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// helloHandlet
type helloHandlet struct {
	// Parent
	Handlet_
	// States
	example string // an example config entry
}

func (h *helloHandlet) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *helloHandlet) OnShutdown() { h.Webapp().DecHandlet() }

func (h *helloHandlet) OnConfigure() {
	// .example
	h.ConfigureString("example", &h.example, nil, "this is default value for example config entry.")
}
func (h *helloHandlet) OnPrepare() {
	m := simple.New() // you can write your own mapper as long as it implements the hemi.Mapper interface

	m.GET("/", h.index)
	m.Map("/foo", h.handleFoo)

	h.UseMapper(h, m) // equip handlet with a mapper so it can call handles automatically through Dispatch()
}

func (h *helloHandlet) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	h.Dispatch(req, resp, h.notFound)
	return true
}
func (h *helloHandlet) notFound(req ServerRequest, resp ServerResponse) {
	resp.Send("oops, target not found!")
}

func (h *helloHandlet) index(req ServerRequest, resp ServerResponse) {
	resp.Send(h.example)
}
func (h *helloHandlet) handleFoo(req ServerRequest, resp ServerResponse) {
	resp.Echo(req.UserAgent())
	resp.Echo(req.T("x"))
	resp.AddTrailer("y", "123")
}

func (h *helloHandlet) GET_abc(req ServerRequest, resp ServerResponse) { // GET /abc
	resp.Send("this is GET /abc")
}
func (h *helloHandlet) GET_def(req ServerRequest, resp ServerResponse) { // GET /def
	resp.Send("this is GET /def")
}
func (h *helloHandlet) POST_def(req ServerRequest, resp ServerResponse) { // POST /def
	resp.Send("this is POST /def")
}
func (h *helloHandlet) GET_cookie(req ServerRequest, resp ServerResponse) { // GET /cookie
	cookie := new(Cookie)
	cookie.Set("name1", "value1")
	resp.AddCookie(cookie)
	resp.Send("this is GET /cookie")
}
