// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is an example app showing how to use Gorox application server to host an app.

package example

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	// Register additional handlers for app.
	RegisterHandler("exampleHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(exampleHandler)
		h.init(name, stage, app)
		return h
	})
	// Register app initializer.
	RegisterAppInit("example", func(app *App) error {
		app.AddSetting("name1", "value1")
		return nil
	})
}

// exampleHandler
type exampleHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage // current stage
	app   *App   // belonging app
	// States
	content string
}

func (h *exampleHandler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app

	m := newExampleMapper()

	m.GET("/", h.handleIndex)
	m.GET("/foo", h.handleFoo)
	m.POST("/bar", h.handleBar)
	m.GET("/baz", h.handleBaz)

	h.UseMapper(h, m)
}

func (h *exampleHandler) OnConfigure() {
	// content
	h.ConfigureString("content", &h.content, nil, "this is example.")
}
func (h *exampleHandler) OnPrepare() {
}
func (h *exampleHandler) OnShutdown() {
	// Do nothing.
}

func (h *exampleHandler) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, nil)
	return // request is handled, next = false
}

func (h *exampleHandler) handleIndex(req Request, resp Response) {
	resp.Send(h.content)
}
func (h *exampleHandler) handleFoo(req Request, resp Response) {
	resp.Send("this is page foo")
}
func (h *exampleHandler) handleBar(req Request, resp Response) {
	resp.Push(req.Content())
	resp.Push(req.T("x"))
	resp.Push(req.T("y"))
	resp.AddTrailer("z", "123")
}
func (h *exampleHandler) handleBaz(req Request, resp Response) {
	resp.Push("aa")
	resp.Push("bb")
	resp.AddTrailer("cc", "dd")
}
func (h *exampleHandler) GET_abc(req Request, resp Response) {
	resp.Send("this is GET /abc")
}
func (h *exampleHandler) POST_def(req Request, resp Response) {
	resp.Send("this is POST /def")
}

// exampleMapper implements hemi.Mapper.
type exampleMapper struct {
	gets    map[string]Handle
	posts   map[string]Handle
	puts    map[string]Handle
	deletes map[string]Handle
}

func newExampleMapper() *exampleMapper {
	m := new(exampleMapper)
	m.gets = make(map[string]Handle)
	m.posts = make(map[string]Handle)
	m.puts = make(map[string]Handle)
	m.deletes = make(map[string]Handle)
	return m
}

func (m *exampleMapper) GET(pattern string, handle Handle) {
	m.gets[pattern] = handle
}
func (m *exampleMapper) POST(pattern string, handle Handle) {
	m.posts[pattern] = handle
}
func (m *exampleMapper) PUT(pattern string, handle Handle) {
	m.puts[pattern] = handle
}
func (m *exampleMapper) DELETE(pattern string, handle Handle) {
	m.deletes[pattern] = handle
}

func (m *exampleMapper) FindHandle(req Request) Handle {
	if path := req.Path(); req.IsGET() {
		return m.gets[path]
	} else if req.IsPOST() {
		return m.posts[path]
	} else if req.IsPUT() {
		return m.puts[path]
	} else if req.IsDELETE() {
		return m.deletes[path]
	} else {
		return nil
	}
}
func (m *exampleMapper) FindMethod(req Request) string {
	path := req.Path()
	return req.Method() + "_" + path[1:] // path always starts with '/'.
}
