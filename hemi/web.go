// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// General Webapp Server implementation.

package hemi

import (
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Hstate is the component interface to storages of HTTP states.
type Hstate interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	Set(sid []byte, sess *Session) error
	Get(sid []byte) (sess *Session, err error)
	Del(sid []byte) error
}

// Hstate_ is a parent.
type Hstate_ struct { // for all hstates
	// Parent
	Component_
	// Assocs
	stage *Stage // current stage
}

func (s *Hstate_) OnCreate(compName string, stage *Stage) {
	s.MakeComp(compName)
	s.stage = stage
}

func (s *Hstate_) Stage() *Stage { return s.stage }

// Session is an HTTP session in hstate.
type Session struct {
	// TODO
	ID      [40]byte // session id
	Secret  [40]byte // secret key
	Created int64    // unix time
	Expires int64    // unix time
	Role    int8     // 0: default, >0: user defined values
	Device  int8     // terminal device type
	state1  int8     // user defined state1
	state2  int8     // user defined state2
	state3  int32    // user defined state3
	states  map[string]string
}

func (s *Session) init() {
	s.states = make(map[string]string)
}

func (s *Session) Get(name string) string        { return s.states[name] }
func (s *Session) Set(name string, value string) { s.states[name] = value }
func (s *Session) Del(name string)               { delete(s.states, name) }

// Webapp is the Web application.
type Webapp struct {
	// Parent
	Component_
	// Mixins
	_accessLogger_
	// Assocs
	stage    *Stage            // current stage
	hstate   Hstate            // the hstate which is used by this webapp
	servers  []HTTPServer      // bound http servers. may be empty
	handlets compDict[Handlet] // defined handlets. indexed by compName
	revisers compDict[Reviser] // defined revisers. indexed by compName
	socklets compDict[Socklet] // defined socklets. indexed by compName
	rules    []*Rule           // defined rules. the order must be kept, so we use list. TODO: use ordered map?
	// States
	hostnames        [][]byte          // like: ("www.example.com", "1.2.3.4", "fff8::1")
	webRoot          string            // root dir for the web
	file404          string            // 404 file path
	text404          []byte            // bytes of the default 404 file
	tlsCertificate   string            // tls certificate file, in pem format
	tlsPrivateKey    string            // tls private key file, in pem format
	maxMultiformSize int64             // max content size when content type is multipart/form-data
	settings         map[string]string // webapp settings defined and used by users
	settingsLock     sync.RWMutex      // protects settings
	isDefault        bool              // is this webapp the default webapp of its belonging http servers?
	exactHostnames   [][]byte          // like: ("example.com")
	suffixHostnames  [][]byte          // like: ("*.example.com")
	prefixHostnames  [][]byte          // like: ("www.example.*")
	revisersByID     [256]Reviser      // for fast searching. position 0 is not used
	numRevisers      uint8             // used number of revisersByID in this webapp
}

func (a *Webapp) onCreate(compName string, stage *Stage) {
	a.MakeComp(compName)
	a.stage = stage
	a.handlets = make(compDict[Handlet])
	a.revisers = make(compDict[Reviser])
	a.socklets = make(compDict[Socklet])
	a.numRevisers = 1 // position 0 is not used
}
func (a *Webapp) OnShutdown() { close(a.ShutChan) } // notifies maintain() which shutdown sub components

func (a *Webapp) OnConfigure() {
	a._accessLogger_.onConfigure(a)

	// .hostnames
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

	// .webRoot
	a.ConfigureString("webRoot", &a.webRoot, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New("webRoot is required")
	}, "")
	a.webRoot = strings.TrimRight(a.webRoot, "/")

	// .file404
	a.ConfigureString("file404", &a.file404, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".file404 has an invalid value")
	}, "")

	// .tlsCertificate
	a.ConfigureString("tlsCertificate", &a.tlsCertificate, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".tlsCertificate has an invalid value")
	}, "")

	// .tlsPrivateKey
	a.ConfigureString("tlsPrivateKey", &a.tlsPrivateKey, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".tlsCertificate has an invalid value")
	}, "")

	// .maxMultiformSize
	a.ConfigureInt64("maxMultiformSize", &a.maxMultiformSize, func(value int64) error {
		if value > 0 && value <= _1T {
			return nil
		}
		return errors.New(".maxMultiformSize has an invalid value")
	}, _128M)

	// .settings
	a.ConfigureStringDict("settings", &a.settings, nil, make(map[string]string))

	// .withHstate
	if v, ok := a.Find("withHstate"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if hstate := a.stage.Hstate(compName); hstate == nil {
				UseExitf("unknown hstate: '%s'\n", compName)
			} else {
				a.hstate = hstate
			}
		} else {
			UseExitln("invalid withHstate")
		}
	}

	// sub components
	a.handlets.walk(Handlet.OnConfigure)
	a.revisers.walk(Reviser.OnConfigure)
	a.socklets.walk(Socklet.OnConfigure)
	for _, rule := range a.rules {
		rule.OnConfigure()
	}
}
func (a *Webapp) OnPrepare() {
	a._accessLogger_.onPrepare(a)

	if a.file404 != "" {
		if data, err := os.ReadFile(a.file404); err == nil {
			a.text404 = data
		}
	}

	// sub components
	a.handlets.walk(Handlet.OnPrepare)
	a.revisers.walk(Reviser.OnPrepare)
	a.socklets.walk(Socklet.OnPrepare)
	for _, rule := range a.rules {
		rule.OnPrepare()
	}

	initsLock.RLock()
	webappInit := webappInits[a.compName]
	initsLock.RUnlock()
	if webappInit != nil {
		if err := webappInit(a); err != nil {
			UseExitln(err.Error())
		}
	}

	if len(a.rules) == 0 {
		Printf("no rules defined for webapp: '%s'\n", a.compName)
	}
}

func (a *Webapp) maintain() { // runner
	a.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})

	a.IncSubs(len(a.handlets) + len(a.revisers) + len(a.socklets) + len(a.rules))
	for _, rule := range a.rules {
		go rule.OnShutdown()
	}
	a.socklets.goWalk(Socklet.OnShutdown)
	a.revisers.goWalk(Reviser.OnShutdown)
	a.handlets.goWalk(Handlet.OnShutdown)
	a.WaitSubs() // handlets, revisers, socklets, rules

	a.CloseLog()
	if DebugLevel() >= 2 {
		Printf("webapp=%s done\n", a.CompName())
	}
	a.stage.DecSub() // webapp
}

func (a *Webapp) createHandlet(compSign string, compName string) Handlet {
	if a.Handlet(compName) != nil {
		UseExitln("conflicting handlet with a same component name in webapp")
	}
	creatorsLock.RLock()
	create, ok := handletCreators[compSign]
	creatorsLock.RUnlock()
	if !ok {
		UseExitln("unknown handlet sign: " + compSign)
	}
	handlet := create(compName, a.stage, a)
	handlet.setShell(handlet)
	a.handlets[compName] = handlet
	return handlet
}
func (a *Webapp) createReviser(compSign string, compName string) Reviser {
	if a.numRevisers == 255 {
		UseExitln("cannot create reviser: too many revisers in one webapp")
	}
	if a.Reviser(compName) != nil {
		UseExitln("conflicting reviser with a same component name in webapp")
	}
	creatorsLock.RLock()
	create, ok := reviserCreators[compSign]
	creatorsLock.RUnlock()
	if !ok {
		UseExitln("unknown reviser sign: " + compSign)
	}
	reviser := create(compName, a.stage, a)
	reviser.setShell(reviser)
	reviser.setID(a.numRevisers)
	a.revisers[compName] = reviser
	a.revisersByID[a.numRevisers] = reviser
	a.numRevisers++
	return reviser
}
func (a *Webapp) createSocklet(compSign string, compName string) Socklet {
	if a.Socklet(compName) != nil {
		UseExitln("conflicting socklet with a same component name in webapp")
	}
	creatorsLock.RLock()
	create, ok := sockletCreators[compSign]
	creatorsLock.RUnlock()
	if !ok {
		UseExitln("unknown socklet sign: " + compSign)
	}
	socklet := create(compName, a.stage, a)
	socklet.setShell(socklet)
	a.socklets[compName] = socklet
	return socklet
}
func (a *Webapp) createRule(compName string) *Rule {
	if a.Rule(compName) != nil {
		UseExitln("conflicting rule with a same component name")
	}
	rule := new(Rule)
	rule.onCreate(compName, a)
	rule.setShell(rule)
	a.rules = append(a.rules, rule)
	return rule
}

func (a *Webapp) bindServer(server HTTPServer) { a.servers = append(a.servers, server) }

func (a *Webapp) Handlet(compName string) Handlet { return a.handlets[compName] }
func (a *Webapp) Reviser(compName string) Reviser { return a.revisers[compName] }
func (a *Webapp) Socklet(compName string) Socklet { return a.socklets[compName] }
func (a *Webapp) Rule(compName string) *Rule {
	for _, rule := range a.rules {
		if rule.compName == compName {
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

func (a *Webapp) dispatchExchan(req ServerRequest, resp ServerResponse) {
	req.makeAbsPath() // for fs check rules, if any
	for _, rule := range a.rules {
		if !rule.isMatch(req) {
			continue
		}
		if handled := rule.executeExchan(req, resp); handled {
			if rule.logAccess {
				a.Logf("status=%d %s %s\n", resp.Status(), req.Method(), req.UnsafeURI())
			}
			return
		}
	}
	// If we reach here, it means the exchan is not handled by any rules or handlets in this webapp.
	resp.SendNotFound(a.text404)
}
func (a *Webapp) dispatchSocket(req ServerRequest, sock ServerSocket) {
	req.makeAbsPath() // for fs check rules, if any
	for _, rule := range a.rules {
		if !rule.isMatch(req) {
			continue
		}
		if served := rule.executeSocket(req, sock); served {
			if rule.logAccess {
				// TODO: log access?
			}
			return
		}
	}
	// If we reach here, it means the socket is not served by any rules or socklets in this webapp.
	sock.Close()
}

// Rule component defines a rule for matching requests and handlets.
type Rule struct {
	// Parent
	Component_
	// Assocs
	webapp   *Webapp   // associated webapp
	handlets []Handlet // handlets in this rule. NOTICE: handlets are sub components of webapp, not rule
	revisers []Reviser // revisers in this rule. NOTICE: revisers are sub components of webapp, not rule
	socklets []Socklet // socklets in this rule. NOTICE: socklets are sub components of webapp, not rule
	// States
	general    bool     // general match?
	logAccess  bool     // enable logging for this rule?
	returnCode int16    // ...
	returnText []byte   // ...
	varCode    int16    // the variable code
	varName    string   // the variable name
	patterns   [][]byte // condition patterns
	regexps    []*regexp.Regexp
	matcher    func(rule *Rule, req ServerRequest, value []byte) bool
}

func (r *Rule) onCreate(compName string, webapp *Webapp) {
	r.MakeComp(compName)
	r.webapp = webapp
}
func (r *Rule) OnShutdown() {
	r.webapp.DecSub() // rule
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

	// .logAccess
	r.ConfigureBool("logAccess", &r.logAccess, true)

	// .returnCode
	r.ConfigureInt16("returnCode", &r.returnCode, func(value int16) error {
		if value >= 200 && value <= 999 {
			return nil
		}
		return errors.New(".returnCode has an invalid value")
	}, 0)

	// .returnText
	r.ConfigureBytes("returnText", &r.returnText, nil, nil)

	// .handlets
	if v, ok := r.Find("handlets"); ok {
		if len(r.socklets) > 0 {
			UseExitln("cannot mix handlets and socklets in a rule")
		}
		if len(r.handlets) > 0 {
			UseExitln("specifying handlets is not allowed while there are literal handlets")
		}
		if compNames, ok := v.StringList(); ok {
			for _, compName := range compNames {
				if handlet := r.webapp.Handlet(compName); handlet != nil {
					r.handlets = append(r.handlets, handlet)
				} else {
					UseExitf("handlet '%s' does not exist\n", compName)
				}
			}
		} else {
			UseExitln("invalid handlet names")
		}
	}

	// .revisers
	if v, ok := r.Find("revisers"); ok {
		if len(r.revisers) != 0 {
			UseExitln("specifying revisers is not allowed while there are literal revisers")
		}
		if compNames, ok := v.StringList(); ok {
			for _, compName := range compNames {
				if reviser := r.webapp.Reviser(compName); reviser != nil {
					r.revisers = append(r.revisers, reviser)
				} else {
					UseExitf("reviser '%s' does not exist\n", compName)
				}
			}
		} else {
			UseExitln("invalid reviser names")
		}
	}

	// .socklets
	if v, ok := r.Find("socklets"); ok {
		if len(r.handlets) > 0 {
			UseExitln("cannot mix socklets and handlets in a rule")
		}
		if len(r.socklets) > 0 {
			UseExitln("specifying socklets is not allowed while there are literal socklets")
		}
		if compNames, ok := v.StringList(); ok {
			for _, compName := range compNames {
				if socklet := r.webapp.Socklet(compName); socklet != nil {
					r.socklets = append(r.socklets, socklet)
				} else {
					UseExitf("socklet '%s' does not exist\n", compName)
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

func (r *Rule) isMatch(req ServerRequest) bool {
	if r.general {
		return true
	}
	varValue := req.unsafeVariable(r.varCode, r.varName)
	return r.matcher(r, req, varValue)
}

var ruleMatchers = map[string]struct {
	matcher func(rule *Rule, req ServerRequest, value []byte) bool
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

func (r *Rule) equalMatch(req ServerRequest, value []byte) bool { // value == patterns
	return equalMatch(value, r.patterns)
}
func (r *Rule) prefixMatch(req ServerRequest, value []byte) bool { // value ^= patterns
	return prefixMatch(value, r.patterns)
}
func (r *Rule) suffixMatch(req ServerRequest, value []byte) bool { // value $= patterns
	return suffixMatch(value, r.patterns)
}
func (r *Rule) containMatch(req ServerRequest, value []byte) bool { // value *= patterns
	return containMatch(value, r.patterns)
}
func (r *Rule) regexpMatch(req ServerRequest, value []byte) bool { // value ~= patterns
	return regexpMatch(value, r.regexps)
}
func (r *Rule) fileMatch(req ServerRequest, value []byte) bool { // value -f
	pathInfo := req.getPathInfo()
	return pathInfo != nil && !pathInfo.IsDir()
}
func (r *Rule) dirMatch(req ServerRequest, value []byte) bool { // value -d
	if len(value) == 1 && value[0] == '/' {
		// webRoot is not included and thus not treated as dir
		return false
	}
	return r.dirMatchWithWebRoot(req, value)
}
func (r *Rule) existMatch(req ServerRequest, value []byte) bool { // value -e
	if len(value) == 1 && value[0] == '/' {
		// webRoot is not included and thus not treated as exist
		return false
	}
	return r.existMatchWithWebRoot(req, value)
}
func (r *Rule) dirMatchWithWebRoot(req ServerRequest, _ []byte) bool { // value -D
	pathInfo := req.getPathInfo()
	return pathInfo != nil && pathInfo.IsDir()
}
func (r *Rule) existMatchWithWebRoot(req ServerRequest, _ []byte) bool { // value -E
	pathInfo := req.getPathInfo()
	return pathInfo != nil
}
func (r *Rule) notEqualMatch(req ServerRequest, value []byte) bool { // value != patterns
	return notEqualMatch(value, r.patterns)
}
func (r *Rule) notPrefixMatch(req ServerRequest, value []byte) bool { // value !^ patterns
	return notPrefixMatch(value, r.patterns)
}
func (r *Rule) notSuffixMatch(req ServerRequest, value []byte) bool { // value !$ patterns
	return notSuffixMatch(value, r.patterns)
}
func (r *Rule) notContainMatch(req ServerRequest, value []byte) bool { // value !* patterns
	return notContainMatch(value, r.patterns)
}
func (r *Rule) notRegexpMatch(req ServerRequest, value []byte) bool { // value !~ patterns
	return notRegexpMatch(value, r.regexps)
}
func (r *Rule) notFileMatch(req ServerRequest, value []byte) bool { // value !f
	pathInfo := req.getPathInfo()
	return pathInfo == nil || pathInfo.IsDir()
}
func (r *Rule) notDirMatch(req ServerRequest, value []byte) bool { // value !d
	pathInfo := req.getPathInfo()
	return pathInfo == nil || !pathInfo.IsDir()
}
func (r *Rule) notExistMatch(req ServerRequest, value []byte) bool { // value !e
	pathInfo := req.getPathInfo()
	return pathInfo == nil
}

func (r *Rule) executeExchan(req ServerRequest, resp ServerResponse) (handled bool) {
	if r.returnCode != 0 {
		resp.SetStatus(r.returnCode)
		if len(r.returnText) == 0 {
			resp.SendBytes(nil)
		} else {
			resp.SendBytes(r.returnText)
		}
		return true
	}

	if len(r.handlets) > 0 { // there are handlets in this rule, so we check against origin server or reverse proxy here.
		toOrigin := true
		for _, handlet := range r.handlets {
			if handlet.IsProxy() { // request to a reverse proxy. checks against reverse proxies
				toOrigin = false
				// Add checks here.
				if handlet.IsCache() { // request to a proxy cache. checks against proxy caches
					// Add checks here.
				}
				break
			}
		}
		if toOrigin { // request to an origin server
			if req.contentIsForm() && req.contentIsEncoded() { // currently a form with content coding is not supported yet
				resp.SendUnsupportedMediaType("", "", nil)
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
func (r *Rule) executeSocket(req ServerRequest, sock ServerSocket) (served bool) {
	// TODO
	/*
		if r.socklet == nil {
			return
		}
		r.socklet.Serve(req, sock)
	*/
	return true
}

// Handlet component handles the incoming request and gives an outgoing response if the request is handled.
type Handlet interface {
	// Imports
	Component
	// Methods
	IsProxy() bool // reverse proxies and origins are different, we must differentiate them
	IsCache() bool // proxy caches and reverse proxies are different, we must differentiate them
	Handle(req ServerRequest, resp ServerResponse) (handled bool)
}

// Handlet_ is a parent.
type Handlet_ struct { // for all handlets
	// Parent
	Component_
	// Assocs
	stage  *Stage  // current stage
	webapp *Webapp // the webapp to which the handlet belongs
	mapper Mapper  // ...
	// States
	rShell reflect.Value // the shell handlet
}

func (h *Handlet_) OnCreate(compName string, stage *Stage, webapp *Webapp) {
	h.MakeComp(compName)
	h.stage = stage
	h.webapp = webapp
}

func (h *Handlet_) Stage() *Stage   { return h.stage }
func (h *Handlet_) Webapp() *Webapp { return h.webapp }

func (h *Handlet_) IsProxy() bool { return false } // override this for reverse proxy handlets
func (h *Handlet_) IsCache() bool { return false } // override this for proxy cache handlets

func (h *Handlet_) UseMapper(handlet Handlet, mapper Mapper) {
	h.mapper = mapper
	h.rShell = reflect.ValueOf(handlet)
}
func (h *Handlet_) Dispatch(req ServerRequest, resp ServerResponse, notFound Handle) {
	if h.mapper != nil {
		if handle := h.mapper.FindHandle(req); handle != nil {
			handle(req, resp)
			return
		}
		if handleName := h.mapper.HandleName(req); handleName != "" {
			if rMethod := h.rShell.MethodByName(handleName); rMethod.IsValid() {
				rMethod.Call([]reflect.Value{reflect.ValueOf(req), reflect.ValueOf(resp)})
				return
			}
		}
	}
	// No handle was found.
	if notFound == nil {
		resp.SendNotFound(nil)
	} else {
		notFound(req, resp)
	}
}

// Handle is a function which handles an http request and gives an http response.
type Handle func(req ServerRequest, resp ServerResponse)

// Mapper performs request mapping in handlets. Mappers are not components.
type Mapper interface {
	FindHandle(req ServerRequest) Handle // called firstly
	HandleName(req ServerRequest) string // called secondly
}

// Reviser component revises incoming requests and outgoing responses.
type Reviser interface {
	// Imports
	Component
	// Methods
	ID() uint8
	setID(id uint8)
	Rank() int8 // 0-31 (with 0-15 as tunable, 16-31 as fixed)

	// For incoming requests, either sized or vague
	BeforeRecv(req ServerRequest, resp ServerResponse)                 // for sized content
	BeforeDraw(req ServerRequest, resp ServerResponse)                 // for vague content
	OnInput(req ServerRequest, resp ServerResponse, input *Chain) bool // for both sized and vague
	FinishDraw(req ServerRequest, resp ServerResponse)                 // for vague content

	// For outgoing responses, either sized or vague
	BeforeSend(req ServerRequest, resp ServerResponse)              // for sized content
	BeforeEcho(req ServerRequest, resp ServerResponse)              // for vague content
	OnOutput(req ServerRequest, resp ServerResponse, output *Chain) // for both sized and vague
	FinishEcho(req ServerRequest, resp ServerResponse)              // for vague content
}

// Reviser_ is a parent.
type Reviser_ struct { // for all revisers
	// Parent
	Component_
	// Assocs
	stage  *Stage  // current stage
	webapp *Webapp // the webapp to which the reviser belongs
	// States
	id uint8 // the reviser id
}

func (r *Reviser_) OnCreate(compName string, stage *Stage, webapp *Webapp) {
	r.MakeComp(compName)
	r.stage = stage
	r.webapp = webapp
}

func (r *Reviser_) Stage() *Stage   { return r.stage }
func (r *Reviser_) Webapp() *Webapp { return r.webapp }

func (r *Reviser_) ID() uint8      { return r.id }
func (r *Reviser_) setID(id uint8) { r.id = id }

// Socklet component handles the webSocket.
type Socklet interface {
	// Imports
	Component
	// Methods
	IsProxy() bool // reverse proxies and origin servers are different, we must differentiate them
	Serve(req ServerRequest, sock ServerSocket)
}

// Socklet_ is a parent.
type Socklet_ struct { // for all socklets
	// Parent
	Component_
	// Assocs
	stage  *Stage  // current stage
	webapp *Webapp // the webapp to which the socklet belongs
	// States
}

func (s *Socklet_) OnCreate(compName string, stage *Stage, webapp *Webapp) {
	s.MakeComp(compName)
	s.stage = stage
	s.webapp = webapp
}

func (s *Socklet_) Stage() *Stage   { return s.stage }
func (s *Socklet_) Webapp() *Webapp { return s.webapp }

func (s *Socklet_) IsProxy() bool { return false } // override this for reverse proxy socklets
