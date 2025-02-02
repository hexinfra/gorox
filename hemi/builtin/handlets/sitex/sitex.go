// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Sitex handlets implement a simple Web application framework.

package sitex

import (
	"bytes"
	"os"
	"reflect"
	"strings"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("sitex", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(Sitex)
		h.OnCreate(compName, stage, webapp)
		return h
	})
}

// Sitex
type Sitex struct {
	// Parent
	Handlet_
	// States
	sites         map[string]*Site // name -> site
	rdbms         string           // relational database
	hostnameSites map[string]*Site // hostname -> site, for routing
}

func (h *Sitex) OnCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
	h.hostnameSites = make(map[string]*Site)
	h.sites = make(map[string]*Site)
}
func (h *Sitex) OnShutdown() {
	h.Webapp().DecSub() // handlet
}

func (h *Sitex) OnConfigure() {
	// .sites
	v, ok := h.Find("sites")
	if !ok {
		UseExitln("sites must be defined")
	}
	vSites, ok := v.Dict()
	if !ok {
		UseExitln("sites must be a dict")
	}
	for siteName, vSite := range vSites {
		siteDict, ok := vSite.Dict()
		if !ok {
			UseExitln("elements of a site must be a dict")
		}
		site := new(Site)
		h.sites[siteName] = site
		site.name = siteName
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
			site.viewDir = TopDir() + "/apps/" + h.Webapp().CompName() + "/" + siteName + "/view"
		}
		site.settings = make(map[string]string)
		if vSettings, ok := siteDict["settings"]; ok {
			if settings, ok := vSettings.StringDict(); ok {
				site.settings = settings
			}
		}
	}

	// .rdbms
	h.ConfigureString("rdbms", &h.rdbms, nil, "")
}
func (h *Sitex) OnPrepare() {
	// TODO
}

func (h *Sitex) RegisterSite(siteName string, pack any) { // called on webapp init.
	if site, ok := h.sites[siteName]; ok {
		site.pack = reflect.TypeOf(pack)
	} else {
		BugExitf("unknown site: %s\n", siteName)
	}
}

func (h *Sitex) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	site := h.hostnameSites[req.Hostname()]
	if site == nil {
		site = h.hostnameSites["*"]
		if site == nil {
			resp.SendNotFound(nil)
			return true
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
		return true
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
	return true
}

// Site
type Site struct {
	name      string
	hostnames []string
	viewDir   string
	settings  map[string]string
	pack      reflect.Type
}

func (s *Site) show(req ServerRequest, resp ServerResponse, page string) {
	if html := s.load(req, s.viewDir+"/"+page+".html"); html == nil {
		resp.SendNotFound(nil)
	} else {
		resp.SendBytes(html)
	}
}
func (s *Site) load(req ServerRequest, htmlFile string) []byte {
	html, err := os.ReadFile(htmlFile)
	if err != nil {
		return nil
	}
	var subs [][]byte
	for {
		i := bytes.Index(html, htmlLL)
		if i == -1 {
			break
		}
		j := bytes.Index(html, htmlRR)
		if j < i {
			break
		}
		subs = append(subs, html[:i])
		i += len(htmlLL)
		token := string(html[i:j])
		if first := token[0]; first == '$' {
			switch token {
			case "$scheme":
				subs = append(subs, []byte(req.Scheme()))
			case "$colonport":
				subs = append(subs, []byte(req.Colonport()))
			case "$uri": // TODO: XSS
				subs = append(subs, []byte(req.URI()))
			default:
				// Do nothing
			}
		} else if first == '@' {
			subs = append(subs, []byte(s.settings[token[1:]]))
		} else {
			subs = append(subs, s.load(req, s.viewDir+"/"+token))
		}
		html = html[j+len(htmlRR):]
	}
	if len(subs) == 0 {
		return html
	}
	subs = append(subs, html)
	return bytes.Join(subs, nil)
}

// Target
type Target struct {
	Site string            // front
	Path string            // /foo/bar
	Args map[string]string // a=b&c=d
}
