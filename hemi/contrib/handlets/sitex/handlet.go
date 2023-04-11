// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Sitex handlets implement a simple MVC Web application framework.

package sitex

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"reflect"
	"strings"
)

func init() {
	RegisterHandlet("sitex", func(name string, stage *Stage, app *App) Handlet {
		h := new(Sitex)
		h.OnCreate(name, stage, app)
		return h
	})
}

// Sitex handlet implements a simple MVC Web application framework.
type Sitex struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
	sites         map[string]*Site // name -> site
	rdbms         string           // relational database
	hostnameSites map[string]*Site // hostname -> site, for routing
}

func (h *Sitex) OnCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
	h.hostnameSites = make(map[string]*Site)
	h.sites = make(map[string]*Site)
}
func (h *Sitex) OnShutdown() {
	h.app.SubDone()
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
		site.name = name
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
		site.settings = make(map[string]string)
		if vSettings, ok := siteDict["settings"]; ok {
			if settings, ok := vSettings.StringDict(); ok {
				site.settings = settings
			}
		}
	}
	// rdbms
	h.ConfigureString("rdbms", &h.rdbms, nil, "")
}
func (h *Sitex) OnPrepare() {
}

func (h *Sitex) RegisterSite(name string, pack any) { // called on app init.
	if site, ok := h.sites[name]; ok {
		site.pack = reflect.TypeOf(pack)
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

	if site.pack == nil {
		site.show(req, resp, page)
		return
	}

	rPack := reflect.New(site.pack)
	rReq, rResp := reflect.ValueOf(req), reflect.ValueOf(resp)
	rPack.MethodByName("Init").Call([]reflect.Value{reflect.ValueOf(site), rReq, rResp, reflect.ValueOf(method), reflect.ValueOf(action)})
	if fn := rPack.MethodByName(method + "_" + action); fn.IsValid() {
		if before := rPack.MethodByName("BeforeAction"); before.IsValid() {
			before.Call(nil)
		}
		fn.Call([]reflect.Value{rReq, rResp})
		if !resp.IsSent() {
			resp.SendBytes(nil)
		}
		if after := rPack.MethodByName("AfterAction"); after.IsValid() {
			after.Call(nil)
		}
	} else {
		site.show(req, resp, page)
	}
	return
}
