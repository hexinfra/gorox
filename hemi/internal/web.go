// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// App and related components.

package internal

import (
	"bytes"
	"github.com/hexinfra/gorox/hemi/libraries/logger"
	"os"
	"strings"
	"sync"
)

// App is the application.
type App struct {
	// Mixins
	Component_
	contentSaver_ // so requests can save their large contents in local file system.
	// Assocs
	stage    *Stage            // current stage
	servers  []httpServer      // linked http servers. may be empty
	handlers compDict[Handler] // defined handlers. indexed by name
	changers compDict[Changer] // defined changers. indexed by name
	revisers compDict[Reviser] // defined revisers. indexed by name
	socklets compDict[Socklet] // defined socklets. indexed by name
	rules    compList[*Rule]   // defined rules. the order must be kept, so we use list. TODO: use ordered map?
	// States
	hostnames            [][]byte          // like: ("www.example.com", "1.2.3.4", "fff8::1")
	webRoot              string            // root dir for the web
	file404              string            // 404 file path
	tlsCertificate       string            // tls certificate file, in pem format
	tlsPrivateKey        string            // tls private key file, in pem format
	errorLog             []string          // (file, rotate)
	accessLog            []string          // (file, rotate)
	maxMemoryContentSize int32             // max content size that can be loaded into memory
	maxUploadContentSize int64             // max content size that uploads files through multipart/form-data
	settings             map[string]string // app settings defined and used by users
	settingsLock         sync.RWMutex      // protects settings
	isDefault            bool              // is this app a default app?
	proxyOnly            bool              // is this app a proxy-only app?
	exactHostnames       [][]byte          // like: ("example.com")
	suffixHostnames      [][]byte          // like: ("*.example.com")
	prefixHostnames      [][]byte          // like: ("www.example.*")
	accessLogger         *logger.Logger    // app access logger
	errorLogger          *logger.Logger    // app error logger
	bytes404             []byte            // bytes of the default 404 file
	changersByID         [256]Changer      // for fast searching. position 0 is not used
	revisersByID         [256]Reviser      // for fast searching. position 0 is not used
	nChangers            uint8             // used number of changersByID in this app
	nRevisers            uint8             // used number of revisersByID in this app
}

func (a *App) init(name string, stage *Stage) {
	a.SetName(name)
	a.stage = stage
	a.handlers = make(compDict[Handler])
	a.changers = make(compDict[Changer])
	a.revisers = make(compDict[Reviser])
	a.socklets = make(compDict[Socklet])
	a.nChangers = 1 // position 0 is not used
	a.nRevisers = 1 // position 0 is not used
}

func (a *App) OnConfigure() {
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
		a.hostnames = nil
	}
	// webRoot
	a.ConfigureString("webRoot", &a.webRoot, func(value string) bool { return value != "" }, "")
	a.webRoot = strings.TrimRight(a.webRoot, "/")
	// file404
	a.ConfigureString("file404", &a.file404, func(value string) bool { return value != "" }, "")
	// tlsCertificate
	a.ConfigureString("tlsCertificate", &a.tlsCertificate, func(value string) bool { return value != "" }, "")
	// tlsPrivateKey
	a.ConfigureString("tlsPrivateKey", &a.tlsPrivateKey, func(value string) bool { return value != "" }, "")
	// errorLog
	if v, ok := a.Find("errorLog"); ok {
		if log, ok := v.StringListN(2); ok {
			a.errorLog = log
		} else {
			UseExitln("invalid errorLog")
		}
	} else {
		a.errorLog = nil
	}
	// accessLog
	if v, ok := a.Find("accessLog"); ok {
		if log, ok := v.StringListN(2); ok {
			a.accessLog = log
		} else {
			UseExitln("invalid accessLog")
		}
	} else {
		a.accessLog = nil
	}
	// maxMemoryContentSize
	a.ConfigureInt32("maxMemoryContentSize", &a.maxMemoryContentSize, func(value int32) bool { return value > 0 && value <= _1G }, _1M) // DO NOT CHANGE THIS, otherwise integer overflow may occur
	// maxUploadContentSize
	a.ConfigureInt64("maxUploadContentSize", &a.maxUploadContentSize, func(value int64) bool { return value > 0 && value <= _1T }, _128M)
	// saveContentFilesDir
	a.ConfigureString("saveContentFilesDir", &a.saveContentFilesDir, func(value string) bool { return value != "" }, TempDir()+"/apps/"+a.name)
	// settings
	a.ConfigureStringDict("settings", &a.settings, nil, make(map[string]string))
	// proxyOnly
	a.ConfigureBool("proxyOnly", &a.proxyOnly, false)

	// sub components
	a.handlers.walk(Handler.OnConfigure)
	a.changers.walk(Changer.OnConfigure)
	a.revisers.walk(Reviser.OnConfigure)
	a.socklets.walk(Socklet.OnConfigure)
	a.rules.walk((*Rule).OnConfigure)
}
func (a *App) OnPrepare() {
	if a.errorLog != nil {
		//a.errorLogger = newLogger(a.errorLog[0], a.errorLog[1])
	}
	if a.accessLog != nil {
		//a.accessLogger = newLogger(a.accessLog[0], a.accessLog[1])
	}
	if a.file404 != "" {
		if data, err := os.ReadFile(a.file404); err == nil {
			a.bytes404 = data
		}
	}
	a.makeContentFilesDir(0755)

	// sub components
	a.handlers.walk(Handler.OnPrepare)
	a.changers.walk(Changer.OnPrepare)
	a.revisers.walk(Reviser.OnPrepare)
	a.socklets.walk(Socklet.OnPrepare)
	a.rules.walk((*Rule).OnPrepare)
}
func (a *App) OnShutdown() {
	// sub components
	a.rules.walk((*Rule).OnShutdown)
	a.socklets.walk(Socklet.OnShutdown)
	a.revisers.walk(Reviser.OnShutdown)
	a.changers.walk(Changer.OnShutdown)
	a.handlers.walk(Handler.OnShutdown)

	// TODO: shutdown a
}

func (a *App) createHandler(sign string, name string) Handler {
	if a.Handler(name) != nil {
		UseExitln("conflicting handler with a same name in app")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := handlerCreators[sign]
	if !ok {
		UseExitln("unknown handler sign: " + sign)
	}
	handler := create(name, a.stage, a)
	if a.proxyOnly && !handler.IsProxy() {
		UseExitln("cannot bind non-proxy handlers to a proxy-only app")
	}
	handler.setShell(handler)
	a.handlers[name] = handler
	return handler
}
func (a *App) createChanger(sign string, name string) Changer {
	if a.nChangers == 255 {
		UseExitln("cannot create changer: too many changers in one app")
	}
	if a.Changer(name) != nil {
		UseExitln("conflicting changer with a same name in app")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := changerCreators[sign]
	if !ok {
		UseExitln("unknown changer sign: " + sign)
	}
	changer := create(name, a.stage, a)
	changer.setShell(changer)
	changer.setID(a.nChangers)
	a.changers[name] = changer
	a.changersByID[a.nChangers] = changer
	a.nChangers++
	return changer
}
func (a *App) createReviser(sign string, name string) Reviser {
	if a.nRevisers == 255 {
		UseExitln("cannot create reviser: too many revisers in one app")
	}
	if a.Reviser(name) != nil {
		UseExitln("conflicting reviser with a same name in app")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := reviserCreators[sign]
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
func (a *App) createSocklet(sign string, name string) Socklet {
	if a.Socklet(name) != nil {
		UseExitln("conflicting socklet with a same name in app")
	}
	creatorsLock.RLock()
	defer creatorsLock.RUnlock()
	create, ok := sockletCreators[sign]
	if !ok {
		UseExitln("unknown socklet sign: " + sign)
	}
	socklet := create(name, a.stage, a)
	if a.proxyOnly && !socklet.IsProxy() {
		UseExitln("cannot bind non-proxy socklets to a proxy-only app")
	}
	socklet.setShell(socklet)
	a.socklets[name] = socklet
	return socklet
}
func (a *App) createRule(name string) *Rule {
	if a.Rule(name) != nil {
		UseExitln("conflicting rule with a same name")
	}
	rule := new(Rule)
	rule.init(name, a)
	rule.setShell(rule)
	a.rules = append(a.rules, rule)
	return rule
}

func (a *App) Handler(name string) Handler { return a.handlers[name] }
func (a *App) Changer(name string) Changer { return a.changers[name] }
func (a *App) Reviser(name string) Reviser { return a.revisers[name] }
func (a *App) Socklet(name string) Socklet { return a.socklets[name] }
func (a *App) Rule(name string) *Rule {
	for _, rule := range a.rules {
		if rule.name == name {
			return rule
		}
	}
	return nil
}

func (a *App) AddSetting(name string, value string) {
	a.settingsLock.Lock()
	a.settings[name] = value
	a.settingsLock.Unlock()
}
func (a *App) Setting(name string) (value string, ok bool) {
	a.settingsLock.RLock()
	value, ok = a.settings[name]
	a.settingsLock.RUnlock()
	return
}

func (a *App) Log(s string) {
	// TODO
}
func (a *App) Logln(s string) {
	// TODO
}
func (a *App) Logf(format string, args ...any) {
	// TODO
	if a.accessLogger != nil {
		//a.accessLogger.logf(format, args...)
	}
}

func (a *App) linkServer(server httpServer) {
	a.servers = append(a.servers, server)
}

func (a *App) start() {
	initsLock.RLock()
	appInit := appInits[a.name]
	initsLock.RUnlock()
	if appInit != nil {
		if err := appInit(a); err != nil {
			BugExitln(err.Error())
		}
	}
	// TODO: wait for shutdown?
}

func (a *App) changerByID(id uint8) Changer { // for fast searching
	return a.changersByID[id]
}
func (a *App) reviserByID(id uint8) Reviser { // for fast searching
	return a.revisersByID[id]
}

func (a *App) dispatchNormal(req Request, resp Response) {
	if a.proxyOnly && req.VersionCode() == Version1_0 {
		resp.setConnectionClose() // A proxy server MUST NOT maintain a persistent connection with an HTTP/1.0 client.
	}
	req.makeAbsPath() // for fs check rules, if any
	for _, rule := range a.rules {
		if !rule.isMatch(req) {
			continue
		}
		if processed := rule.executeNormal(req, resp); processed {
			if rule.log && a.accessLogger != nil {
				//a.accessLogger.logf("status=%d %s %s\n", resp.Status(), req.Method(), req.UnsafeURI())
			}
			return
		}
	}
	// If we reach here, it means the stream is not processed by any rule in this app.
	resp.SendNotFound(a.bytes404)
}
func (a *App) dispatchSocket(req Request, sock Socket) {
	req.makeAbsPath() // for fs check rules, if any
	for _, rule := range a.rules {
		if !rule.isMatch(req) {
			continue
		}
		rule.executeSocket(req, sock)
		// TODO: log?
		return
	}
	// If we reach here, it means the socket is not processed by any rule in this app.
	sock.Close()
}

// Handler component handles the incoming request and gives an outgoing response if the request is handled.
type Handler interface {
	Component
	IsProxy() bool // proxies and origins are different, we must differentiate them
	IsCache() bool // caches and proxies are different, we must differentiate them
	Handle(req Request, resp Response) (next bool)
}

// Handler_ is the mixin for all handlers.
type Handler_ struct {
	// Mixins
	Component_
	// States
}

func (h *Handler_) IsProxy() bool { return false } // override this for proxy handlers
func (h *Handler_) IsCache() bool { return false } // override this for cache handlers

// Handle is a function which can handle http request and gives http response.
type Handle func(req Request, resp Response)

// Changer component changes the incoming request.
type Changer interface {
	Component
	ider

	Rank() int8 // 0-31 (with 0-15 for fixed, 16-31 for user)

	BeforeRecv(req Request, resp Response) // for identity content

	BeforePull(req Request, resp Response) // for chunked content
	FinishPull(req Request, resp Response) // for chunked content

	Change(req Request, resp Response, chain Chain) Chain
}

// Changer_ is the mixin for all changers.
type Changer_ struct {
	// Mixins
	Component_
	ider_
	// States
}

// Reviser component revises the outgoing response.
type Reviser interface {
	Component
	ider

	Rank() int8 // 0-31 (with 0-15 for user, 16-31 for fixed)

	BeforeSend(req Request, resp Response) // for identity content

	BeforePush(req Request, resp Response) // for chunked content
	FinishPush(req Request, resp Response) // for chunked content

	Revise(req Request, resp Response, chain Chain) Chain
}

// Reviser_ is the mixin for all revisers.
type Reviser_ struct {
	// Mixins
	Component_
	ider_
	// States
}

// Socklet component handles the websocket.
type Socklet interface {
	Component
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
	app      *App      // belonging app
	handlers []Handler // handlers in this rule
	changers []Changer // changers in this rule
	revisers []Reviser // revisers in this rule
	socklets []Socklet // socklets in this rule
	// States
	general    bool     // general match?
	log        bool     // enable logging for this rule?
	returnCode int16    // ...
	returnText []byte   // ...
	varCode    int16    // the variable code
	patterns   [][]byte // condition patterns
	matcher    func(rule *Rule, req Request, value []byte) bool
}

func (r *Rule) init(name string, app *App) {
	r.SetName(name)
	r.app = app
}

func (r *Rule) OnConfigure() {
	if r.info == nil {
		r.general = true
	} else {
		cond := r.info.(ruleCond)
		r.varCode = cond.varCode
		for _, pattern := range cond.patterns {
			if pattern == "" {
				UseExitln("empty rule cond pattern")
			}
			r.patterns = append(r.patterns, []byte(pattern))
		}
		if matcher, ok := ruleMatchers[cond.compare]; ok {
			r.matcher = matcher.matcher
			if matcher.fsCheck && r.app.webRoot == "" {
				UseExitln("can't do fs check since app's webRoot is empty. you must set webRoot for app")
			}
		} else {
			UseExitln("unknown compare in rule condition")
		}
	}

	// log
	r.ConfigureBool("log", &r.log, true)

	// returnCode
	r.ConfigureInt16("returnCode", &r.returnCode, func(value int16) bool { return value >= 200 && value < 1000 }, 0)

	// returnText
	var returnText string
	r.ConfigureString("returnText", &returnText, nil, "")
	r.returnText = []byte(returnText)

	// handlers
	if v, ok := r.Find("handlers"); ok {
		if len(r.socklets) > 0 {
			UseExitln("cannot mix handlers and socklets in a rule")
		}
		if len(r.handlers) > 0 {
			UseExitln("specifying handlers is not allowed while there are literal handlers")
		}
		if names, ok := v.StringList(); ok {
			for _, name := range names {
				if handler := r.app.Handler(name); handler != nil {
					r.handlers = append(r.handlers, handler)
				} else {
					UseExitf("handler '%s' does not exist\n", name)
				}
			}
		} else {
			UseExitln("invalid handler names")
		}
	}
	// changers
	if v, ok := r.Find("changers"); ok {
		if len(r.changers) != 0 {
			UseExitln("specifying changers is not allowed while there are literal changers")
		}
		if names, ok := v.StringList(); ok {
			for _, name := range names {
				if changer := r.app.Changer(name); changer != nil {
					r.changers = append(r.changers, changer)
				} else {
					UseExitf("changer '%s' does not exist\n", name)
				}
			}
		} else {
			UseExitln("invalid changer names")
		}
	}
	// revisers
	if v, ok := r.Find("revisers"); ok {
		if len(r.revisers) != 0 {
			UseExitln("specifying revisers is not allowed while there are literal revisers")
		}
		if names, ok := v.StringList(); ok {
			for _, name := range names {
				if reviser := r.app.Reviser(name); reviser != nil {
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
		if len(r.handlers) > 0 {
			UseExitln("cannot mix socklets and handlers in a rule")
		}
		if len(r.socklets) > 0 {
			UseExitln("specifying socklets is not allowed while there are literal socklets")
		}
		if names, ok := v.StringList(); ok {
			for _, name := range names {
				if socklet := r.app.Socklet(name); socklet != nil {
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
func (r *Rule) OnShutdown() {
}

func (r *Rule) addHandler(handler Handler) { r.handlers = append(r.handlers, handler) }
func (r *Rule) addChanger(changer Changer) { r.changers = append(r.changers, changer) }
func (r *Rule) addReviser(reviser Reviser) { r.revisers = append(r.revisers, reviser) }
func (r *Rule) addSocklet(socklet Socklet) { r.socklets = append(r.socklets, socklet) }

func (r *Rule) isMatch(req Request) bool {
	if r.general {
		return true
	}
	return r.matcher(r, req, req.unsafeVariable(r.varCode))
}

var ruleMatchers = map[string]struct {
	matcher func(rule *Rule, req Request, value []byte) bool
	fsCheck bool
}{
	"==": {(*Rule).equalMatch, false},
	"^=": {(*Rule).prefixMatch, false},
	"$=": {(*Rule).suffixMatch, false},
	"*=": {(*Rule).wildcardMatch, false},
	"~=": {(*Rule).regexpMatch, false},
	"-f": {(*Rule).fileMatch, true},
	"-d": {(*Rule).dirMatch, true},
	"-e": {(*Rule).existMatch, true},
	"-D": {(*Rule).dirMatchWithWebRoot, true},
	"-E": {(*Rule).existMatchWithWebRoot, true},
	"!=": {(*Rule).notEqualMatch, false},
	"!^": {(*Rule).notPrefixMatch, false},
	"!$": {(*Rule).notSuffixMatch, false},
	"!*": {(*Rule).notWildcardMatch, false},
	"!~": {(*Rule).notRegexpMatch, false},
	"!f": {(*Rule).notFileMatch, true},
	"!d": {(*Rule).notDirMatch, true},
	"!e": {(*Rule).notExistMatch, true},
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
func (r *Rule) wildcardMatch(req Request, value []byte) bool { // value *= patterns
	// TODO(diogin): implementation
	return false
}
func (r *Rule) regexpMatch(req Request, value []byte) bool { // value ~= patterns
	// TODO(diogin): implementation
	return false
}
func (r *Rule) fileMatch(req Request, value []byte) bool { // value -f
	pathInfo := req.getPathInfo()
	return pathInfo.Valid() && !pathInfo.IsDir()
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
	return pathInfo.Valid() && pathInfo.IsDir()
}
func (r *Rule) existMatchWithWebRoot(req Request, _ []byte) bool { // value -E
	pathInfo := req.getPathInfo()
	return pathInfo.Valid()
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
func (r *Rule) notWildcardMatch(req Request, value []byte) bool { // value !* patterns
	// TODO(diogin): implementation
	return true
}
func (r *Rule) notRegexpMatch(req Request, value []byte) bool { // value !~ patterns
	// TODO(diogin): implementation
	return true
}
func (r *Rule) notFileMatch(req Request, value []byte) bool { // value !f
	pathInfo := req.getPathInfo()
	return !pathInfo.Valid() || pathInfo.IsDir()
}
func (r *Rule) notDirMatch(req Request, value []byte) bool { // value !d
	pathInfo := req.getPathInfo()
	return !pathInfo.Valid() || !pathInfo.IsDir()
}
func (r *Rule) notExistMatch(req Request, value []byte) bool { // value !e
	pathInfo := req.getPathInfo()
	return !pathInfo.Valid()
}

func (r *Rule) executeNormal(req Request, resp Response) (processed bool) {
	if r.returnCode != 0 {
		resp.SetStatus(r.returnCode)
		if len(r.returnText) == 0 {
			resp.SendBytes(nil)
		} else {
			resp.SendBytes(r.returnText)
		}
		return true
	}

	if len(r.handlers) > 0 { // there are handlers in this rule, so we check against origin server or proxy server here.
		toOrigin := true
		for _, handler := range r.handlers {
			if handler.IsProxy() { // request to proxy server. checks against proxy server
				toOrigin = false
				if req.VersionCode() == Version1_0 {
					resp.setConnectionClose() // A proxy server MUST NOT maintain a persistent connection with an HTTP/1.0 client.
				}
				if handler.IsCache() { // request to proxy cache. checks against proxy cache
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
	// Hook changers on request. When receiving request content, these changers will be executed.
	for _, changer := range r.changers {
		req.hookChanger(changer)
	}
	// Hook revisers on response. When sending response content, these revisers will be executed.
	for _, reviser := range r.revisers {
		resp.hookReviser(reviser)
	}
	// Execute handlers
	for _, handler := range r.handlers {
		if next := handler.Handle(req, resp); !next {
			return true
		}
	}
	return false
}
func (r *Rule) executeSocket(req Request, sock Socket) {
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
}
