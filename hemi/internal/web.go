// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Web application and related components.

package internal

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Webapp is the Web application.
type Webapp struct {
	// Mixins
	Component_
	contentSaver_ // so requests can save their large contents in local file system.
	// Assocs
	stage    *Stage            // current stage
	stater   Stater            // the stater which is used by this webapp
	servers  []webServer       // bound web servers. may be empty
	handlets compDict[Handlet] // defined handlets. indexed by name
	revisers compDict[Reviser] // defined revisers. indexed by name
	socklets compDict[Socklet] // defined socklets. indexed by name
	rules    compList[*Rule]   // defined rules. the order must be kept, so we use list. TODO: use ordered map?
	// States
	hostnames            [][]byte          // like: ("www.example.com", "1.2.3.4", "fff8::1")
	webRoot              string            // root dir for the web
	file404              string            // 404 file path
	text404              []byte            // bytes of the default 404 file
	tlsCertificate       string            // tls certificate file, in pem format
	tlsPrivateKey        string            // tls private key file, in pem format
	accessLog            *logcfg           // ...
	logger               *logger           // webapp access logger
	maxMemoryContentSize int32             // max content size that can be loaded into memory
	maxUploadContentSize int64             // max content size that uploads files through multipart/form-data
	settings             map[string]string // webapp settings defined and used by users
	settingsLock         sync.RWMutex      // protects settings
	isDefault            bool              // is this webapp a default webapp?
	proxyOnly            bool              // is this webapp a proxy-only webapp?
	exactHostnames       [][]byte          // like: ("example.com")
	suffixHostnames      [][]byte          // like: ("*.example.com")
	prefixHostnames      [][]byte          // like: ("www.example.*")
	revisersByID         [256]Reviser      // for fast searching. position 0 is not used
	nRevisers            uint8             // used number of revisersByID in this webapp
}

func (a *Webapp) onCreate(name string, stage *Stage) {
	a.MakeComp(name)
	a.stage = stage
	a.handlets = make(compDict[Handlet])
	a.revisers = make(compDict[Reviser])
	a.socklets = make(compDict[Socklet])
	a.nRevisers = 1 // position 0 is not used
}
func (a *Webapp) OnShutdown() {
	close(a.ShutChan)
}

func (a *Webapp) OnConfigure() {
	a.contentSaver_.onConfigure(a, TmpsDir()+"/web/webapps/"+a.name)

	// hostnames
	if v, ok := a.Find("hostnames"); ok {
		hostnames, ok := v.StringList()
		if !ok || len(hostnames) == 0 {
			UseExitln("empty hostnames")
		}
		for _, hostname := range hostnames {
			if hostname == "" {
				UseExitln("cannot contain empty hostname in hostnames")
			}
			if hostname == "*" {
				a.isDefault = true
				continue
			}
			h := []byte(hostname)
			a.hostnames = append(a.hostnames, h)
			n, p := len(h), -1
			for i := 0; i < n; i++ {
				if h[i] != '*' {
					continue
				}
				if p == -1 {
					p = i
					break
				} else {
					UseExitln("currently only one wildcard is allowed")
				}
			}
			if p == -1 {
				a.exactHostnames = append(a.exactHostnames, h)
			} else if p == 0 {
				a.suffixHostnames = append(a.suffixHostnames, h[1:])
			} else if p == n-1 {
				a.prefixHostnames = append(a.prefixHostnames, h[:n-1])
			} else {
				UseExitln("surrounded matching is not supported yet")
			}
		}
	} else {
		UseExitln("webapp.hostnames is required")
	}

	// webRoot
	a.ConfigureString("webRoot", &a.webRoot, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New("webRoot is required")
	}, "")
	a.webRoot = strings.TrimRight(a.webRoot, "/")

	// file404
	a.ConfigureString("file404", &a.file404, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".file404 has an invalid value")
	}, "")

	// tlsCertificate
	a.ConfigureString("tlsCertificate", &a.tlsCertificate, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".tlsCertificate has an invalid value")
	}, "")

	// tlsPrivateKey
	a.ConfigureString("tlsPrivateKey", &a.tlsPrivateKey, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".tlsCertificate has an invalid value")
	}, "")

	// accessLog, TODO

	// maxMemoryContentSize
	a.ConfigureInt32("maxMemoryContentSize", &a.maxMemoryContentSize, func(value int32) error {
		if value > 0 && value <= _1G {
			return nil
		}
		return errors.New(".maxMemoryContentSize has an invalid value")
	}, _16M) // DO NOT CHANGE THIS, otherwise integer overflow may occur

	// maxUploadContentSize
	a.ConfigureInt64("maxUploadContentSize", &a.maxUploadContentSize, func(value int64) error {
		if value > 0 && value <= _1T {
			return nil
		}
		return errors.New(".maxUploadContentSize has an invalid value")
	}, _128M)

	// settings
	a.ConfigureStringDict("settings", &a.settings, nil, make(map[string]string))
	// proxyOnly
	a.ConfigureBool("proxyOnly", &a.proxyOnly, false)
	if a.proxyOnly {
		for _, handlet := range a.handlets {
			if !handlet.IsProxy() {
				UseExitln("cannot bind non-proxy handlets to a proxy-only webapp")
			}
		}
		for _, socklet := range a.socklets {
			if !socklet.IsProxy() {
				UseExitln("cannot bind non-proxy socklets to a proxy-only webapp")
			}
		}
	}

	// withStater
	if v, ok := a.Find("withStater"); ok {
		if name, ok := v.String(); ok && name != "" {
			if stater := a.stage.Stater(name); stater == nil {
				UseExitf("unknown stater: '%s'\n", name)
			} else {
				a.stater = stater
			}
		} else {
			UseExitln("invalid withStater")
		}
	}

	// sub components
	a.handlets.walk(Handlet.OnConfigure)
	a.revisers.walk(Reviser.OnConfigure)
	a.socklets.walk(Socklet.OnConfigure)
	a.rules.walk((*Rule).OnConfigure)
}
func (a *Webapp) OnPrepare() {
	a.contentSaver_.onPrepare(a, 0755)

	if a.accessLog != nil {
		//a.logger = newLogger(a.accessLog.logFile, a.accessLog.rotate)
	}
	if a.file404 != "" {
		if data, err := os.ReadFile(a.file404); err == nil {
			a.text404 = data
		}
	}

	// sub components
	a.handlets.walk(Handlet.OnPrepare)
	a.revisers.walk(Reviser.OnPrepare)
	a.socklets.walk(Socklet.OnPrepare)
	a.rules.walk((*Rule).OnPrepare)

	initsLock.RLock()
	webappInit := webappInits[a.name]
	initsLock.RUnlock()
	if webappInit != nil {
		if err := webappInit(a); err != nil {
			UseExitln(err.Error())
		}
	}

	if len(a.rules) == 0 {
		Printf("no rules defined for webapp: '%s'\n", a.name)
	}
}

func (a *Webapp) createHandlet(sign string, name string) Handlet {
	if a.Handlet(name) != nil {
		UseExitln("conflicting handlet with a same name in webapp")
	}
	creatorsLock.RLock()
	create, ok := handletCreators[sign]
	creatorsLock.RUnlock()
	if !ok {
		UseExitln("unknown handlet sign: " + sign)
	}
	handlet := create(name, a.stage, a)
	handlet.setShell(handlet)
	a.handlets[name] = handlet
	return handlet
}
func (a *Webapp) createReviser(sign string, name string) Reviser {
	if a.nRevisers == 255 {
		UseExitln("cannot create reviser: too many revisers in one webapp")
	}
	if a.Reviser(name) != nil {
		UseExitln("conflicting reviser with a same name in webapp")
	}
	creatorsLock.RLock()
	create, ok := reviserCreators[sign]
	creatorsLock.RUnlock()
	if !ok {
		UseExitln("unknown reviser sign: " + sign)
	}
	reviser := create(name, a.stage, a)
	reviser.setShell(reviser)
	reviser.setID(a.nRevisers)
	a.revisers[name] = reviser
	a.revisersByID[a.nRevisers] = reviser
	a.nRevisers++
	return reviser
}
func (a *Webapp) createSocklet(sign string, name string) Socklet {
	if a.Socklet(name) != nil {
		UseExitln("conflicting socklet with a same name in webapp")
	}
	creatorsLock.RLock()
	create, ok := sockletCreators[sign]
	creatorsLock.RUnlock()
	if !ok {
		UseExitln("unknown socklet sign: " + sign)
	}
	socklet := create(name, a.stage, a)
	socklet.setShell(socklet)
	a.socklets[name] = socklet
	return socklet
}
func (a *Webapp) createRule(name string) *Rule {
	if a.Rule(name) != nil {
		UseExitln("conflicting rule with a same name")
	}
	rule := new(Rule)
	rule.onCreate(name, a)
	rule.setShell(rule)
	a.rules = append(a.rules, rule)
	return rule
}

func (a *Webapp) Handlet(name string) Handlet { return a.handlets[name] }
func (a *Webapp) Reviser(name string) Reviser { return a.revisers[name] }
func (a *Webapp) Socklet(name string) Socklet { return a.socklets[name] }
func (a *Webapp) Rule(name string) *Rule {
	for _, rule := range a.rules {
		if rule.name == name {
			return rule
		}
	}
	return nil
}

func (a *Webapp) reviserByID(id uint8) Reviser { return a.revisersByID[id] }

func (a *Webapp) AddSetting(name string, value string) {
	a.settingsLock.Lock()
	a.settings[name] = value
	a.settingsLock.Unlock()
}
func (a *Webapp) Setting(name string) (value string, ok bool) {
	a.settingsLock.RLock()
	value, ok = a.settings[name]
	a.settingsLock.RUnlock()
	return
}

func (a *Webapp) Log(str string) {
	if a.logger != nil {
		a.logger.Log(str)
	}
}
func (a *Webapp) Logln(str string) {
	if a.logger != nil {
		a.logger.Logln(str)
	}
}
func (a *Webapp) Logf(format string, args ...any) {
	if a.logger != nil {
		a.logger.Logf(format, args...)
	}
}

func (a *Webapp) bindServer(server webServer) { a.servers = append(a.servers, server) }

func (a *Webapp) maintain() { // goroutine
	a.Loop(time.Second, func(now time.Time) {
		// TODO
	})

	a.IncSub(len(a.handlets) + len(a.revisers) + len(a.socklets) + len(a.rules))
	a.rules.goWalk((*Rule).OnShutdown)
	a.socklets.goWalk(Socklet.OnShutdown)
	a.revisers.goWalk(Reviser.OnShutdown)
	a.handlets.goWalk(Handlet.OnShutdown)
	a.WaitSubs() // handlets, revisers, socklets, rules

	if a.logger != nil {
		a.logger.Close()
	}
	if Debug() >= 2 {
		Printf("webapp=%s done\n", a.Name())
	}
	a.stage.SubDone()
}

func (a *Webapp) dispatchHandlet(req Request, resp Response) {
	if a.proxyOnly && req.VersionCode() == Version1_0 {
		resp.setConnectionClose() // A proxy server MUST NOT maintain a persistent connection with an HTTP/1.0 client.
	}
	req.makeAbsPath() // for fs check rules, if any
	for _, rule := range a.rules {
		if !rule.isMatch(req) {
			continue
		}
		if processed := rule.executeExchan(req, resp); processed {
			if rule.logAccess && a.logger != nil {
				//a.logger.logf("status=%d %s %s\n", resp.Status(), req.Method(), req.UnsafeURI())
			}
			return
		}
	}
	// If we reach here, it means the stream is not processed by any rule in this webapp.
	resp.SendNotFound(a.text404)
}
func (a *Webapp) dispatchSocklet(req Request, sock Socket) {
	req.makeAbsPath() // for fs check rules, if any
	for _, rule := range a.rules {
		if !rule.isMatch(req) {
			continue
		}
		if processed := rule.executeSocket(req, sock); processed {
			if rule.logAccess && a.logger != nil {
				// TODO: log?
			}
			return
		}
	}
	// If we reach here, it means the socket is not processed by any rule in this webapp.
	sock.Close()
}

// Handle is a function which handles web request and gives web response.
type Handle func(req Request, resp Response)

// Router performs request mapping in handlets.
type Router interface {
	FindHandle(req Request) Handle // firstly
	HandleName(req Request) string // secondly
}

// Handlet component handles the incoming request and gives an outgoing response if the request is handled.
type Handlet interface {
	// Imports
	Component
	// Methods
	IsProxy() bool // proxies and origins are different, we must differentiate them
	IsCache() bool // caches and proxies are different, we must differentiate them
	Handle(req Request, resp Response) (handled bool)
}

// Handlet_ is the mixin for all handlets.
type Handlet_ struct {
	// Mixins
	Component_
	// Assocs
	router Router
	// States
	rShell reflect.Value // the shell handlet
}

func (h *Handlet_) IsProxy() bool { return false } // override this for proxy handlets
func (h *Handlet_) IsCache() bool { return false } // override this for cache handlets

func (h *Handlet_) UseRouter(handlet Handlet, router Router) {
	h.router = router
	h.rShell = reflect.ValueOf(handlet)
}
func (h *Handlet_) Dispatch(req Request, resp Response, notFound Handle) {
	if h.router != nil {
		if handle := h.router.FindHandle(req); handle != nil {
			handle(req, resp)
			return
		}
		if name := h.router.HandleName(req); name != "" {
			if rMethod := h.rShell.MethodByName(name); rMethod.IsValid() {
				rMethod.Call([]reflect.Value{reflect.ValueOf(req), reflect.ValueOf(resp)})
				return
			}
		}
	}
	if notFound == nil {
		resp.SendNotFound(nil)
	} else {
		notFound(req, resp)
	}
}

// Reviser component revises incoming requests and outgoing responses.
type Reviser interface {
	// Imports
	Component
	identifiable
	// Methods
	Rank() int8 // 0-31 (with 0-15 as tunable, 16-31 as fixed)
	// TODO
	BeforeRecv(req Request, resp Response) // for sized content
	OnRecv(req Request, resp Response, chain Chain) (Chain, bool)
	BeforeDraw(req Request, resp Response) // for unsized content
	OnDraw(req Request, resp Response, chain Chain) (Chain, bool)
	FinishDraw(req Request, resp Response) // for unsized content
	// TODO
	BeforeSend(req Request, resp Response) // for sized content
	OnSend(req Request, resp Response, content *Chain)
	BeforeEcho(req Request, resp Response) // for unsized content
	OnEcho(req Request, resp Response, chunks *Chain)
	FinishEcho(req Request, resp Response) // for unsized content
}

// Reviser_ is the mixin for all revisers.
type Reviser_ struct {
	// Mixins
	Component_
	identifiable_
	// States
}

// Socklet component handles the websocket.
type Socklet interface {
	// Imports
	Component
	// Methods
	IsProxy() bool // proxys and origins are different, we must differentiate them
	Serve(req Request, sock Socket)
}

// Socklet_ is the mixin for all socklets.
type Socklet_ struct {
	// Mixins
	Component_
	// States
}

func (s *Socklet_) IsProxy() bool { return false } // override this for proxy socklets

// Rule component
type Rule struct {
	// Mixins
	Component_
	// Assocs
	webapp   *Webapp   // associated webapp
	handlets []Handlet // handlets in this rule. NOTICE: handlets are sub components of webapp, not rule
	revisers []Reviser // revisers in this rule. NOTICE: revisers are sub components of webapp, not rule
	socklets []Socklet // socklets in this rule. NOTICE: socklets are sub components of webapp, not rule
	// States
	general    bool     // general match?
	logAccess  bool     // enable booking for this rule?
	returnCode int16    // ...
	returnText []byte   // ...
	varCode    int16    // the variable code
	varName    string   // the variable name
	patterns   [][]byte // condition patterns
	regexps    []*regexp.Regexp
	matcher    func(rule *Rule, req Request, value []byte) bool
}

func (r *Rule) onCreate(name string, webapp *Webapp) {
	r.MakeComp(name)
	r.webapp = webapp
}
func (r *Rule) OnShutdown() {
	r.webapp.SubDone()
}

func (r *Rule) OnConfigure() {
	if r.info == nil {
		r.general = true
	} else {
		cond := r.info.(ruleCond)
		r.varCode = cond.varCode
		r.varName = cond.varName
		isRegexp := cond.compare == "~=" || cond.compare == "!~"
		for _, pattern := range cond.patterns {
			if pattern == "" {
				UseExitln("empty rule cond pattern")
			}
			if !isRegexp {
				r.patterns = append(r.patterns, []byte(pattern))
			} else if exp, err := regexp.Compile(pattern); err == nil {
				r.regexps = append(r.regexps, exp)
			} else {
				UseExitln(err.Error())
			}
		}
		if matcher, ok := ruleMatchers[cond.compare]; ok {
			r.matcher = matcher.matcher
			if matcher.fsCheck && r.webapp.webRoot == "" {
				UseExitln("can't do fs check since webapp's webRoot is empty. you must set webRoot for the webapp")
			}
		} else {
			UseExitln("unknown compare in rule condition")
		}
	}

	// logAccess
	r.ConfigureBool("logAccess", &r.logAccess, true)
	// returnCode
	r.ConfigureInt16("returnCode", &r.returnCode, func(value int16) error {
		if value >= 200 && value < 1000 {
			return nil
		}
		return errors.New(".returnCode has an invalid value")
	}, 0)

	// returnText
	r.ConfigureBytes("returnText", &r.returnText, nil, nil)

	// handlets
	if v, ok := r.Find("handlets"); ok {
		if len(r.socklets) > 0 {
			UseExitln("cannot mix handlets and socklets in a rule")
		}
		if len(r.handlets) > 0 {
			UseExitln("specifying handlets is not allowed while there are literal handlets")
		}
		if names, ok := v.StringList(); ok {
			for _, name := range names {
				if handlet := r.webapp.Handlet(name); handlet != nil {
					r.handlets = append(r.handlets, handlet)
				} else {
					UseExitf("handlet '%s' does not exist\n", name)
				}
			}
		} else {
			UseExitln("invalid handlet names")
		}
	}

	// revisers
	if v, ok := r.Find("revisers"); ok {
		if len(r.revisers) != 0 {
			UseExitln("specifying revisers is not allowed while there are literal revisers")
		}
		if names, ok := v.StringList(); ok {
			for _, name := range names {
				if reviser := r.webapp.Reviser(name); reviser != nil {
					r.revisers = append(r.revisers, reviser)
				} else {
					UseExitf("reviser '%s' does not exist\n", name)
				}
			}
		} else {
			UseExitln("invalid reviser names")
		}
	}

	// socklets
	if v, ok := r.Find("socklets"); ok {
		if len(r.handlets) > 0 {
			UseExitln("cannot mix socklets and handlets in a rule")
		}
		if len(r.socklets) > 0 {
			UseExitln("specifying socklets is not allowed while there are literal socklets")
		}
		if names, ok := v.StringList(); ok {
			for _, name := range names {
				if socklet := r.webapp.Socklet(name); socklet != nil {
					r.socklets = append(r.socklets, socklet)
				} else {
					UseExitf("socklet '%s' does not exist\n", name)
				}
			}
		} else {
			UseExitln("invalid socklet names")
		}
	}
}
func (r *Rule) OnPrepare() {
}

func (r *Rule) addHandlet(handlet Handlet) { r.handlets = append(r.handlets, handlet) }
func (r *Rule) addReviser(reviser Reviser) { r.revisers = append(r.revisers, reviser) }
func (r *Rule) addSocklet(socklet Socklet) { r.socklets = append(r.socklets, socklet) }

func (r *Rule) isMatch(req Request) bool {
	if r.general {
		return true
	}
	value := req.unsafeVariable(r.varCode, r.varName)
	return r.matcher(r, req, value)
}

func (r *Rule) executeExchan(req Request, resp Response) (processed bool) {
	if r.returnCode != 0 {
		resp.SetStatus(r.returnCode)
		if len(r.returnText) == 0 {
			resp.SendBytes(nil)
		} else {
			resp.SendBytes(r.returnText)
		}
		return true
	}

	if len(r.handlets) > 0 { // there are handlets in this rule, so we check against origin server or proxy server here.
		toOrigin := true
		for _, handlet := range r.handlets {
			if handlet.IsProxy() { // request to proxy server. checks against proxy server
				toOrigin = false
				if req.VersionCode() == Version1_0 {
					resp.setConnectionClose() // A proxy server MUST NOT maintain a persistent connection with an HTTP/1.0 client.
				}
				if handlet.IsCache() { // request to proxy cache. checks against proxy cache
					// Add checks here.
				}
				break
			}
		}
		if toOrigin { // request to origin server
			methodCode := req.MethodCode()
			/*
				if methodCode == 0 { // unrecognized request method
					// RFC 9110:
					// An origin server that receives a request method that is unrecognized or not
					// implemented SHOULD respond with the 501 (Not Implemented) status code.
					resp.SendNotImplemented(nil)
					return true
				}
			*/
			if methodCode == MethodPUT && req.HasHeader("content-range") {
				// RFC 9110:
				// An origin server SHOULD respond with a 400 (Bad Request) status code
				// if it receives Content-Range on a PUT for a target resource that
				// does not support partial PUT requests.
				resp.SendBadRequest(nil)
				return true
			}
			// TODO: other general checks against origin server
		}
	}
	// Hook revisers on request and response. When receiving or sending content, these revisers will be executed.
	for _, reviser := range r.revisers { // hook revisers
		req.hookReviser(reviser)
		resp.hookReviser(reviser)
	}
	// Execute handlets
	for _, handlet := range r.handlets {
		if handled := handlet.Handle(req, resp); handled { // request is handled and a response is sent
			return true
		}
	}
	return false
}
func (r *Rule) executeSocket(req Request, sock Socket) (processed bool) {
	// TODO
	/*
		if r.socklet == nil {
			return
		}
		if r.socklet.IsProxy() {
			// TODO
		} else {
			// TODO
		}
		r.socklet.Serve(req, sock)
	*/
	return true
}

func (r *Rule) equalMatch(req Request, value []byte) bool { // value == patterns
	for _, pattern := range r.patterns {
		if bytes.Equal(value, pattern) {
			return true
		}
	}
	return false
}
func (r *Rule) prefixMatch(req Request, value []byte) bool { // value ^= patterns
	for _, pattern := range r.patterns {
		if bytes.HasPrefix(value, pattern) {
			return true
		}
	}
	return false
}
func (r *Rule) suffixMatch(req Request, value []byte) bool { // value $= patterns
	for _, pattern := range r.patterns {
		if bytes.HasSuffix(value, pattern) {
			return true
		}
	}
	return false
}
func (r *Rule) containMatch(req Request, value []byte) bool { // value *= patterns
	for _, pattern := range r.patterns {
		if bytes.Contains(value, pattern) {
			return true
		}
	}
	return false
}
func (r *Rule) regexpMatch(req Request, value []byte) bool { // value ~= patterns
	for _, regexp := range r.regexps {
		if regexp.Match(value) {
			return true
		}
	}
	return false
}
func (r *Rule) fileMatch(req Request, value []byte) bool { // value -f
	pathInfo := req.getPathInfo()
	return pathInfo != nil && !pathInfo.IsDir()
}
func (r *Rule) dirMatch(req Request, value []byte) bool { // value -d
	if len(value) == 1 && value[0] == '/' {
		// webRoot is not included and thus not treated as dir
		return false
	}
	return r.dirMatchWithWebRoot(req, value)
}
func (r *Rule) existMatch(req Request, value []byte) bool { // value -e
	if len(value) == 1 && value[0] == '/' {
		// webRoot is not included and thus not treated as exist
		return false
	}
	return r.existMatchWithWebRoot(req, value)
}
func (r *Rule) dirMatchWithWebRoot(req Request, _ []byte) bool { // value -D
	pathInfo := req.getPathInfo()
	return pathInfo != nil && pathInfo.IsDir()
}
func (r *Rule) existMatchWithWebRoot(req Request, _ []byte) bool { // value -E
	pathInfo := req.getPathInfo()
	return pathInfo != nil
}
func (r *Rule) notEqualMatch(req Request, value []byte) bool { // value != patterns
	for _, pattern := range r.patterns {
		if bytes.Equal(value, pattern) {
			return false
		}
	}
	return true
}
func (r *Rule) notPrefixMatch(req Request, value []byte) bool { // value !^ patterns
	for _, pattern := range r.patterns {
		if bytes.HasPrefix(value, pattern) {
			return false
		}
	}
	return true
}
func (r *Rule) notSuffixMatch(req Request, value []byte) bool { // value !$ patterns
	for _, pattern := range r.patterns {
		if bytes.HasSuffix(value, pattern) {
			return false
		}
	}
	return true
}
func (r *Rule) notContainMatch(req Request, value []byte) bool { // value !* patterns
	for _, pattern := range r.patterns {
		if bytes.Contains(value, pattern) {
			return false
		}
	}
	return true
}
func (r *Rule) notRegexpMatch(req Request, value []byte) bool { // value !~ patterns
	for _, regexp := range r.regexps {
		if regexp.Match(value) {
			return false
		}
	}
	return true
}
func (r *Rule) notFileMatch(req Request, value []byte) bool { // value !f
	pathInfo := req.getPathInfo()
	return pathInfo == nil || pathInfo.IsDir()
}
func (r *Rule) notDirMatch(req Request, value []byte) bool { // value !d
	pathInfo := req.getPathInfo()
	return pathInfo == nil || !pathInfo.IsDir()
}
func (r *Rule) notExistMatch(req Request, value []byte) bool { // value !e
	pathInfo := req.getPathInfo()
	return pathInfo == nil
}

var ruleMatchers = map[string]struct {
	matcher func(rule *Rule, req Request, value []byte) bool
	fsCheck bool
}{
	"==": {(*Rule).equalMatch, false},
	"^=": {(*Rule).prefixMatch, false},
	"$=": {(*Rule).suffixMatch, false},
	"*=": {(*Rule).containMatch, false},
	"~=": {(*Rule).regexpMatch, false},
	"-f": {(*Rule).fileMatch, true},
	"-d": {(*Rule).dirMatch, true},
	"-e": {(*Rule).existMatch, true},
	"-D": {(*Rule).dirMatchWithWebRoot, true},
	"-E": {(*Rule).existMatchWithWebRoot, true},
	"!=": {(*Rule).notEqualMatch, false},
	"!^": {(*Rule).notPrefixMatch, false},
	"!$": {(*Rule).notSuffixMatch, false},
	"!*": {(*Rule).notContainMatch, false},
	"!~": {(*Rule).notRegexpMatch, false},
	"!f": {(*Rule).notFileMatch, true},
	"!d": {(*Rule).notDirMatch, true},
	"!e": {(*Rule).notExistMatch, true},
}
