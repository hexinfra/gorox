// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Sitex handlers implement a simple MVC web application framework.

package sitex

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"reflect"
	"strings"
)

func init() {
	RegisterHandler("sitex", func(name string, stage *Stage, app *App) Handler {
		h := new(Sitex)
		h.Init(name, stage, app)
		return h
	})
}

// Sitex handler implements a simple MVC web application framework.
type Sitex struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
	sites         map[string]*Site // name -> site
	rdbms         string           // relational database
	hostnameSites map[string]*Site // hostname -> site, for routing
}

func (h *Sitex) Init(name string, stage *Stage, app *App) {
	h.Handler_.Init(name, h)
	h.stage = stage
	h.app = app
	h.hostnameSites = make(map[string]*Site)
	h.sites = make(map[string]*Site)
}

func (h *Sitex) OnConfigure() {
	// sites
	v, ok := h.Find("sites")
	if !ok {
		UseExitln("sites must be defined")
	}
	vSites, ok := v.Dict()
	if !ok {
		UseExitln("sites must be a dict")
	}
	for name, vSite := range vSites {
		siteDict, ok := vSite.Dict()
		if !ok {
			UseExitln("elements of a site must be a dict")
		}
		site := new(Site)
		h.sites[name] = site
		vHostnames, ok := siteDict["hostnames"]
		if !ok {
			UseExitln("hostnames is required for sites in sitex")
		}
		if hostnames, ok := vHostnames.StringList(); ok && len(hostnames) > 0 {
			site.hostnames = hostnames
			for _, hostname := range hostnames {
				h.hostnameSites[hostname] = site
			}
		} else {
			UseExitln("in sitex, hostnames must be string list")
		}
		if v, ok := siteDict["viewDir"]; ok {
			if viewDir, ok := v.String(); ok && viewDir != "" {
				site.viewDir = viewDir
			} else {
				UseExitln("viewDir must be string")
			}
		} else {
			site.viewDir = BaseDir() + "/apps/" + h.app.Name() + "/" + name + "/view"
		}
	}
	// rdbms
	h.ConfigureString("rdbms", &h.rdbms, nil, "")
}
func (h *Sitex) OnPrepare() {
}
func (h *Sitex) OnShutdown() {
}

func (h *Sitex) RegisterSite(name string, controller any) { // called on app init.
	if site, ok := h.sites[name]; ok {
		site.controller = reflect.TypeOf(controller)
	} else {
		BugExitf("unknown site: %s\n", name)
	}
}

func (h *Sitex) Stage() *Stage { return h.stage }
func (h *Sitex) App() *App     { return h.app }

func (h *Sitex) Handle(req Request, resp Response) (next bool) {
	site := h.hostnameSites[req.Hostname()]
	if site == nil {
		site = h.hostnameSites["*"]
		if site == nil {
			resp.SendNotFound(nil)
			return
		}
	}

	method := req.Method()
	if method == "HEAD" {
		method = "GET"
	}
	action := "index"
	page := action
	if path := req.Path(); path != "/" {
		path = path[1:]
		if path[len(path)-1] == '/' {
			path += "index"
		}
		action = strings.Replace(path, "/", "_", -1)
		page = strings.Replace(path, "/", "-", -1)
	}

	if site.controller == nil {
		site.show(req, resp, page)
		return
	}

	rController := reflect.New(site.controller)
	rReq, rResp := reflect.ValueOf(req), reflect.ValueOf(resp)
	rController.MethodByName("Init").Call([]reflect.Value{reflect.ValueOf(site), rReq, rResp, reflect.ValueOf(method), reflect.ValueOf(action)})
	if fn := rController.MethodByName(method + "_" + action); fn.IsValid() {
		if before := rController.MethodByName("BeforeAction"); before.IsValid() {
			before.Call(nil)
		}
		fn.Call([]reflect.Value{rReq, rResp})
		if !resp.IsSent() {
			resp.SendBytes(nil)
		}
		if after := rController.MethodByName("AfterAction"); after.IsValid() {
			after.Call(nil)
		}
	} else {
		site.show(req, resp, page)
	}
	return
}
