// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Web application related components.

package internal

import (
	"bytes"
	"errors"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Stater component is the interface to storages of HTTP states. See RFC 6265.
type Stater interface {
	Component
	Maintain() // goroutine
	Set(sid []byte, session *Session)
	Get(sid []byte) (session *Session)
	Del(sid []byte) bool
}

// Stater_
type Stater_ struct {
	// Mixins
	Component_
}

// Session is an HTTP session in stater
type Session struct {
	// TODO
	ID     [40]byte // session id
	Secret [40]byte // secret
	Role   int8     // 0: default, >0: app defined values
	Device int8     // terminal device type
	state1 int8     // app defined state1
	state2 int8     // app defined state2
	state3 int32    // app defined state3
	expire int64    // unix stamp
	states map[string]string
}

func (s *Session) init() {
	s.states = make(map[string]string)
}

func (s *Session) Get(name string) string        { return s.states[name] }
func (s *Session) Set(name string, value string) { s.states[name] = value }
func (s *Session) Del(name string)               { delete(s.states, name) }

// App is the application.
type App struct {
	// Mixins
	Component_
	contentSaver_ // so requests can save their large contents in local file system.
	// Assocs
	stage    *Stage            // current stage
	stater   Stater            // the stater which is used by this app
	servers  []httpServer      // linked http servers. may be empty
	handlets compDict[Handlet] // defined handlets. indexed by name
	revisers compDict[Reviser] // defined revisers. indexed by name
	socklets compDict[Socklet] // defined socklets. indexed by name
	rules    compList[*Rule]   // defined rules. the order must be kept, so we use list. TODO: use ordered map?
	// States
	hostnames            [][]byte          // like: ("www.example.com", "1.2.3.4", "fff8::1")
	webRoot              string            // root dir for the web
	file404              string            // 404 file path
	tlsCertificate       string            // tls certificate file, in pem format
	tlsPrivateKey        string            // tls private key file, in pem format
	accessLog            []string          // (file, rotate)
	logFormat            string            // log format
	booker               *booker           // app access booker
	maxMemoryContentSize int32             // max content size that can be loaded into memory
	maxUploadContentSize int64             // max content size that uploads files through multipart/form-data
	settings             map[string]string // app settings defined and used by users
	settingsLock         sync.RWMutex      // protects settings
	isDefault            bool              // is this app a default app?
	proxyOnly            bool              // is this app a proxy-only app?
	exactHostnames       [][]byte          // like: ("example.com")
	suffixHostnames      [][]byte          // like: ("*.example.com")
	prefixHostnames      [][]byte          // like: ("www.example.*")
	bytes404             []byte            // bytes of the default 404 file
	revisersByID         [256]Reviser      // for fast searching. position 0 is not used
	nRevisers            uint8             // used number of revisersByID in this app
}

func (a *App) onCreate(name string, stage *Stage) {
	a.CompInit(name)
	a.stage = stage
	a.handlets = make(compDict[Handlet])
	a.revisers = make(compDict[Reviser])
	a.socklets = make(compDict[Socklet])
	a.nRevisers = 1 // position 0 is not used
}
func (a *App) OnShutdown() {
	close(a.Shut)
}

func (a *App) OnConfigure() {
	a.contentSaver_.onConfigure(a, TempDir()+"/apps/"+a.name)

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
	// logFormat
	a.ConfigureString("logFormat", &a.logFormat, func(value string) bool { return value != "" }, "%T... todo")
	// maxMemoryContentSize
	a.ConfigureInt32("maxMemoryContentSize", &a.maxMemoryContentSize, func(value int32) bool { return value > 0 && value <= _1G }, _1M) // DO NOT CHANGE THIS, otherwise integer overflow may occur
	// maxUploadContentSize
	a.ConfigureInt64("maxUploadContentSize", &a.maxUploadContentSize, func(value int64) bool { return value > 0 && value <= _1T }, _128M)
	// settings
	a.ConfigureStringDict("settings", &a.settings, nil, make(map[string]string))
	// proxyOnly
	a.ConfigureBool("proxyOnly", &a.proxyOnly, false)
	if a.proxyOnly {
		for _, handlet := range a.handlets {
			if !handlet.IsProxy() {
				UseExitln("cannot bind non-proxy handlets to a proxy-only app")
			}
		}
		for _, socklet := range a.socklets {
			if !socklet.IsProxy() {
				UseExitln("cannot bind non-proxy socklets to a proxy-only app")
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
func (a *App) OnPrepare() {
	a.contentSaver_.onPrepare(a, 0755)

	if a.accessLog != nil {
		//a.booker = newBooker(a.accessLog[0], a.accessLog[1])
	}
	if a.file404 != "" {
		if data, err := os.ReadFile(a.file404); err == nil {
			a.bytes404 = data
		}
	}

	// sub components
	a.handlets.walk(Handlet.OnPrepare)
	a.revisers.walk(Reviser.OnPrepare)
	a.socklets.walk(Socklet.OnPrepare)
	a.rules.walk((*Rule).OnPrepare)

	initsLock.RLock()
	appInit := appInits[a.name]
	initsLock.RUnlock()
	if appInit != nil {
		if err := appInit(a); err != nil {
			UseExitln(err.Error())
		}
	}
}

func (a *App) createHandlet(sign string, name string) Handlet {
	if a.Handlet(name) != nil {
		UseExitln("conflicting handlet with a same name in app")
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
func (a *App) createReviser(sign string, name string) Reviser {
	if a.nRevisers == 255 {
		UseExitln("cannot create reviser: too many revisers in one app")
	}
	if a.Reviser(name) != nil {
		UseExitln("conflicting reviser with a same name in app")
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
func (a *App) createSocklet(sign string, name string) Socklet {
	if a.Socklet(name) != nil {
		UseExitln("conflicting socklet with a same name in app")
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
func (a *App) createRule(name string) *Rule {
	if a.Rule(name) != nil {
		UseExitln("conflicting rule with a same name")
	}
	rule := new(Rule)
	rule.onCreate(name, a)
	rule.setShell(rule)
	a.rules = append(a.rules, rule)
	return rule
}

func (a *App) Handlet(name string) Handlet { return a.handlets[name] }
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
	if a.booker != nil {
		//a.booker.log(s)
	}
}
func (a *App) Logln(s string) {
	// TODO
	if a.booker != nil {
		//a.booker.logln(s)
	}
}
func (a *App) Logf(format string, args ...any) {
	// TODO
	if a.booker != nil {
		//a.booker.logf(format, args...)
	}
}

func (a *App) linkServer(server httpServer) {
	a.servers = append(a.servers, server)
}

func (a *App) maintain() { // goroutine
	Loop(time.Second, a.Shut, func(now time.Time) {
		// TODO
	})
	a.IncSub(len(a.handlets) + len(a.revisers) + len(a.socklets) + len(a.rules))
	a.rules.goWalk((*Rule).OnShutdown)
	a.socklets.goWalk(Socklet.OnShutdown)
	a.revisers.goWalk(Reviser.OnShutdown)
	a.handlets.goWalk(Handlet.OnShutdown)
	a.WaitSubs() // handlets, revisers, socklets, rules
	if a.booker != nil {
		// TODO: close access log file
	}
	if IsDebug(2) {
		Debugf("app=%s done\n", a.Name())
	}
	a.stage.SubDone()
}

func (a *App) reviserByID(id uint8) Reviser { // for fast searching
	return a.revisersByID[id]
}

func (a *App) dispatchHandlet(req Request, resp Response) {
	if a.proxyOnly && req.VersionCode() == Version1_0 {
		resp.setConnectionClose() // A proxy server MUST NOT maintain a persistent connection with an HTTP/1.0 client.
	}
	req.makeAbsPath() // for fs check rules, if any
	for _, rule := range a.rules {
		if !rule.isMatch(req) {
			continue
		}
		if processed := rule.executeNormal(req, resp); processed {
			if rule.logAccess && a.booker != nil {
				//a.booker.logf("status=%d %s %s\n", resp.Status(), req.Method(), req.UnsafeURI())
			}
			return
		}
	}
	// If we reach here, it means the stream is not processed by any rule in this app.
	resp.SendNotFound(a.bytes404)
}
func (a *App) dispatchSocklet(req Request, sock Socket) {
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

// Handlet component handles the incoming request and gives an outgoing response if the request is handled.
type Handlet interface {
	Component
	IsProxy() bool // proxies and origins are different, we must differentiate them
	IsCache() bool // caches and proxies are different, we must differentiate them
	Handle(req Request, resp Response) (next bool)
}

// Handlet_ is the mixin for all handlets.
type Handlet_ struct {
	// Mixins
	Component_
	// States
	rShell reflect.Value // the shell handlet
	router Router
}

func (h *Handlet_) UseRouter(shell any, router Router) {
	h.rShell = reflect.ValueOf(shell)
	h.router = router
}

func (h *Handlet_) Dispatch(req Request, resp Response, notFound Handle) {
	if h.router != nil {
		found := false
		if handle := h.router.FindHandle(req); handle != nil {
			handle(req, resp)
			found = true
		} else if name := h.router.CreateName(req); name != "" {
			if rMethod := h.rShell.MethodByName(name); rMethod.IsValid() {
				rMethod.Call([]reflect.Value{reflect.ValueOf(req), reflect.ValueOf(resp)})
				found = true
			}
		}
		if found {
			return
		}
	}
	if notFound == nil {
		resp.SendNotFound(nil)
	} else {
		notFound(req, resp)
	}
}

func (h *Handlet_) IsProxy() bool { return false } // override this for proxy handlets
func (h *Handlet_) IsCache() bool { return false } // override this for cache handlets

// Handle is a function which can handle http request and gives http response.
type Handle func(req Request, resp Response)

// Reviser component revises incoming requests and outgoing responses.
type Reviser interface {
	Component
	ider

	Rank() int8 // 0-31 (with 0-15 for user, 16-31 for fixed)

	BeforeRecv(req Request, resp Response) // for counted content
	BeforePull(req Request, resp Response) // for chunked content
	FinishPull(req Request, resp Response) // for chunked content
	OnInput(req Request, resp Response, chain Chain) Chain

	BeforeSend(req Request, resp Response) // for counted content
	BeforePush(req Request, resp Response) // for chunked content
	FinishPush(req Request, resp Response) // for chunked content
	OnOutput(req Request, resp Response, chain Chain) Chain
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
	app      *App      // associated app
	handlets []Handlet // handlets in this rule. NOTICE: handlets are sub components of app, not rule
	revisers []Reviser // revisers in this rule. NOTICE: revisers are sub components of app, not rule
	socklets []Socklet // socklets in this rule. NOTICE: socklets are sub components of app, not rule
	// States
	general    bool     // general match?
	logAccess  bool     // enable booking for this rule?
	returnCode int16    // ...
	returnText []byte   // ...
	varCode    int16    // the variable code
	patterns   [][]byte // condition patterns
	matcher    func(rule *Rule, req Request, value []byte) bool
}

func (r *Rule) onCreate(name string, app *App) {
	r.CompInit(name)
	r.app = app
}
func (r *Rule) OnShutdown() {
	r.app.SubDone()
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

	// logAccess
	r.ConfigureBool("logAccess", &r.logAccess, true)

	// returnCode
	r.ConfigureInt16("returnCode", &r.returnCode, func(value int16) bool { return value >= 200 && value < 1000 }, 0)

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
				if handlet := r.app.Handlet(name); handlet != nil {
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
		if len(r.handlets) > 0 {
			UseExitln("cannot mix socklets and handlets in a rule")
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

func (r *Rule) addHandlet(handlet Handlet) {
	r.handlets = append(r.handlets, handlet)
}
func (r *Rule) addReviser(reviser Reviser) {
	r.revisers = append(r.revisers, reviser)
}
func (r *Rule) addSocklet(socklet Socklet) {
	r.socklets = append(r.socklets, socklet)
}

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
	for _, reviser := range r.revisers {
		req.hookReviser(reviser)
		resp.hookReviser(reviser)
	}
	// Execute handlets
	for _, handlet := range r.handlets {
		if next := handlet.Handle(req, resp); !next {
			return true
		}
	}
	return false
}
func (r *Rule) executeSocket(req Request, sock Socket) (processed bool) {
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

// Upload is a file uploaded by client.
type Upload struct { // 48 bytes
	hash     uint16 // hash of name, to support fast comparison
	flags    uint8  // see upload flags
	errCode  int8   // error code
	nameSize uint8  // name size
	baseSize uint8  // base size
	typeSize uint8  // type size
	pathSize uint8  // path size
	nameFrom int32  // like: "avatar"
	baseFrom int32  // like: "michael.jpg"
	typeFrom int32  // like: "image/jpeg"
	pathFrom int32  // like: "/path/to/391384576"
	size     int64  // file size
	meta     string // cannot use []byte as it can cause memory leak if caller save file to another place
}

func (u *Upload) nameEqualString(p []byte, x string) bool {
	if int(u.nameSize) != len(x) {
		return false
	}
	if u.metaSet() {
		return u.meta[u.nameFrom:u.nameFrom+int32(u.nameSize)] == x
	}
	return string(p[u.nameFrom:u.nameFrom+int32(u.nameSize)]) == x
}

const ( // upload flags
	uploadFlagMetaSet = 0b10000000
	uploadFlagIsMoved = 0b01000000
)

func (u *Upload) setMeta(p []byte) {
	if u.flags&uploadFlagMetaSet > 0 {
		return
	}
	u.flags |= uploadFlagMetaSet
	from := u.nameFrom
	if u.baseFrom < from {
		from = u.baseFrom
	}
	if u.pathFrom < from {
		from = u.pathFrom
	}
	if u.typeFrom < from {
		from = u.typeFrom
	}
	max, edge := u.typeFrom, u.typeFrom+int32(u.typeSize)
	if u.pathFrom > max {
		max = u.pathFrom
		edge = u.pathFrom + int32(u.pathSize)
	}
	if u.baseFrom > max {
		max = u.baseFrom
		edge = u.baseFrom + int32(u.baseSize)
	}
	if u.nameFrom > max {
		max = u.nameFrom
		edge = u.nameFrom + int32(u.nameSize)
	}
	u.meta = string(p[from:edge]) // dup to avoid memory leak
	u.nameFrom -= from
	u.baseFrom -= from
	u.typeFrom -= from
	u.pathFrom -= from
}
func (u *Upload) metaSet() bool { return u.flags&uploadFlagMetaSet > 0 }
func (u *Upload) setMoved()     { u.flags |= uploadFlagIsMoved }
func (u *Upload) isMoved() bool { return u.flags&uploadFlagIsMoved > 0 }

const ( // upload error codes
	uploadOK        = 0
	uploadError     = 1
	uploadCantWrite = 2
	uploadTooLarge  = 3
	uploadPartial   = 4
	uploadNoFile    = 5
)

var uploadErrors = [...]error{
	nil, // no error
	errors.New("general error"),
	errors.New("cannot write"),
	errors.New("too large"),
	errors.New("partial"),
	errors.New("no file"),
}

func (u *Upload) IsOK() bool   { return u.errCode == 0 }
func (u *Upload) Error() error { return uploadErrors[u.errCode] }

func (u *Upload) Name() string { return u.meta[u.nameFrom : u.nameFrom+int32(u.nameSize)] }
func (u *Upload) Base() string { return u.meta[u.baseFrom : u.baseFrom+int32(u.baseSize)] }
func (u *Upload) Type() string { return u.meta[u.typeFrom : u.typeFrom+int32(u.typeSize)] }
func (u *Upload) Path() string { return u.meta[u.pathFrom : u.pathFrom+int32(u.pathSize)] }
func (u *Upload) Size() int64  { return u.size }

func (u *Upload) MoveTo(path string) error {
	// TODO
	return nil
}

// Router performs request routing in handlets.
type Router interface {
	FindHandle(req Request) Handle
	CreateName(req Request) string
}

// simpleRouter implements Router.
type simpleRouter struct {
	routes  map[string]Handle // for all methods
	gets    map[string]Handle
	posts   map[string]Handle
	puts    map[string]Handle
	deletes map[string]Handle
}

// NewSimpleRouter creates a simpleRouter.
func NewSimpleRouter() *simpleRouter {
	r := new(simpleRouter)
	r.routes = make(map[string]Handle)
	r.gets = make(map[string]Handle)
	r.posts = make(map[string]Handle)
	r.puts = make(map[string]Handle)
	r.deletes = make(map[string]Handle)
	return r
}

func (r *simpleRouter) Route(path string, handle Handle) {
	r.routes[path] = handle
}

func (r *simpleRouter) GET(path string, handle Handle) {
	r.gets[path] = handle
}
func (r *simpleRouter) POST(path string, handle Handle) {
	r.posts[path] = handle
}
func (r *simpleRouter) PUT(path string, handle Handle) {
	r.puts[path] = handle
}
func (r *simpleRouter) DELETE(path string, handle Handle) {
	r.deletes[path] = handle
}

func (r *simpleRouter) FindHandle(req Request) Handle {
	// TODO
	path := req.Path()
	if handle, ok := r.routes[path]; ok {
		return handle
	} else if req.IsGET() {
		return r.gets[path]
	} else if req.IsPOST() {
		return r.posts[path]
	} else if req.IsPUT() {
		return r.puts[path]
	} else if req.IsDELETE() {
		return r.deletes[path]
	} else {
		return nil
	}
}
func (r *simpleRouter) CreateName(req Request) string {
	method := req.UnsafeMethod()
	path := req.UnsafePath() // always starts with '/'
	name := req.UnsafeMake(len(method) + len(path))
	n := copy(name, method)
	copy(name[n:], path)
	for i := n; i < len(name); i++ {
		if name[i] == '/' {
			name[i] = '_'
		}
	}
	return risky.WeakString(name)
}
