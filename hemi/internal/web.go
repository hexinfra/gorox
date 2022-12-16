// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Web related components.

package internal

import (
	"bytes"
	"errors"
	"fmt"
	"net"
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

// Cacher component is the interface to storages of HTTP caching. See RFC 9111.
type Cacher interface {
	Component
	Maintain() // goroutine
	Set(key []byte, hobject *Hobject)
	Get(key []byte) (hobject *Hobject)
	Del(key []byte) bool
}

// Cacher_
type Cacher_ struct {
	// Mixins
	Component_
}

// Hobject is an HTTP object in cacher
type Hobject struct {
	// TODO
	uri      []byte
	headers  any
	content  any
	trailers any
}

// App is the application.
type App struct {
	// Mixins
	Component_
	contentSaver_ // so requests can save their large contents in local file system.
	// Assocs
	stage    *Stage            // current stage
	stater   Stater            // the stater which is used by this app
	servers  []httpServer      // linked http servers. may be empty
	handlers compDict[Handler] // defined handlers. indexed by name
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
	a.handlers = make(compDict[Handler])
	a.revisers = make(compDict[Reviser])
	a.socklets = make(compDict[Socklet])
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
	if a.proxyOnly {
		for _, handler := range a.handlers {
			if !handler.IsProxy() {
				UseExitln("cannot bind non-proxy handlers to a proxy-only app")
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
	a.handlers.walk(Handler.OnConfigure)
	a.revisers.walk(Reviser.OnConfigure)
	a.socklets.walk(Socklet.OnConfigure)
	a.rules.walk((*Rule).OnConfigure)
}
func (a *App) OnPrepare() {
	if a.accessLog != nil {
		//a.booker = newBooker(a.accessLog[0], a.accessLog[1])
	}
	if a.file404 != "" {
		if data, err := os.ReadFile(a.file404); err == nil {
			a.bytes404 = data
		}
	}
	a.makeContentFilesDir(0755)

	// sub components
	a.handlers.walk(Handler.OnPrepare)
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

func (a *App) OnShutdown() {
	a.Shutdown()
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
	handler.setShell(handler)
	a.handlers[name] = handler
	return handler
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

func (a *App) Handler(name string) Handler {
	return a.handlers[name]
}
func (a *App) Reviser(name string) Reviser {
	return a.revisers[name]
}
func (a *App) Socklet(name string) Socklet {
	return a.socklets[name]
}
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
	a.IncSub(len(a.handlers) + len(a.socklets) + len(a.revisers) + len(a.rules))
	a.rules.goWalk((*Rule).OnShutdown)
	a.socklets.goWalk(Socklet.OnShutdown)
	a.revisers.goWalk(Reviser.OnShutdown)
	a.handlers.goWalk(Handler.OnShutdown)
	a.WaitSubs() // handlers, socklets, revisers, rules
	// TODO: close access log file
	if Debug(2) {
		fmt.Printf("app=%s done\n", a.Name())
	}
	a.stage.SubDone()
}

func (a *App) reviserByID(id uint8) Reviser { // for fast searching
	return a.revisersByID[id]
}

func (a *App) dispatchHandler(req Request, resp Response) {
	if a.proxyOnly && req.VersionCode() == Version1_0 {
		resp.setConnectionClose() // A proxy server MUST NOT maintain a persistent connection with an HTTP/1.0 client.
	}
	req.makeAbsPath() // for fs check rules, if any
	for _, rule := range a.rules {
		if !rule.isMatch(req) {
			continue
		}
		if processed := rule.executeNormal(req, resp); processed {
			if rule.book && a.booker != nil {
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

// Request is the server-side HTTP request and is the interface for *http[1-3]Request.
type Request interface {
	PeerAddr() net.Addr
	App() *App

	VersionCode() uint8
	Version() string // HTTP/1.0, HTTP/1.1, HTTP/2, HTTP/3

	SchemeCode() uint8 // SchemeHTTP, SchemeHTTPS
	IsHTTP() bool
	IsHTTPS() bool
	Scheme() string // http, https

	MethodCode() uint32
	Method() string
	IsGET() bool
	IsPOST() bool
	IsPUT() bool
	IsDELETE() bool

	IsAbsoluteForm() bool
	IsAsteriskOptions() bool

	Authority() string // hostname[:port]
	Hostname() string  // hostname
	ColonPort() string // :port

	URI() string         // /encodedPath?queryString
	Path() string        // /path
	EncodedPath() string // /encodedPath

	QueryString() string // including '?' if query string exists
	Q(name string) string
	QQ(name string, defaultValue string) string
	Qint(name string, defaultValue int) int
	Query(name string) (value string, ok bool)
	QueryList(name string) (list []string, ok bool)
	Queries() (queries [][2]string)
	HasQuery(name string) bool
	AddQuery(name string, value string) bool
	DelQuery(name string) (deleted bool)

	H(name string) string
	HH(name string, defaultValue string) string
	Header(name string) (value string, ok bool)
	HeaderList(name string) (list []string, ok bool)
	Headers() (headers [][2]string)
	HasHeader(name string) bool
	AddHeader(name string, value string) bool
	DelHeader(name string) (deleted bool)

	UserAgent() string
	ContentType() string
	ContentSize() int64
	AcceptTrailers() bool

	TestConditions(modTime int64, etag []byte, asOrigin bool) (status int16, pass bool) // to test preconditons intentionally
	TestIfRanges(modTime int64, etag []byte, asOrigin bool) (pass bool)                 // to test preconditons intentionally

	C(name string) string
	CC(name string, defaultValue string) string
	Cookie(name string) (value string, ok bool)
	CookieList(name string) (list []string, ok bool)
	Cookies() (cookies [][2]string)
	HasCookie(name string) bool
	AddCookie(name string, value string) bool
	DelCookie(name string) (deleted bool)

	HasContent() bool                // contentSize=0 and contentSize=-2 are considered as true
	SetMaxRecvSeconds(seconds int64) // to defend against slowloris attack
	Content() string

	F(name string) string
	FF(name string, defaultValue string) string
	Fint(name string, defaultValue int) int
	Form(name string) (value string, ok bool)
	FormList(name string) (list []string, ok bool)
	Forms() (forms [][2]string)
	HasForm(name string) bool

	U(name string) *Upload
	Upload(name string) (upload *Upload, ok bool)
	UploadList(name string) (list []*Upload, ok bool)
	Uploads() (uploads []*Upload)
	HasUpload(name string) bool

	T(name string) string
	TT(name string, defaultValue string) string
	Trailer(name string) (value string, ok bool)
	TrailerList(name string) (list []string, ok bool)
	Trailers() (trailers [][2]string)
	HasTrailer(name string) bool
	AddTrailer(name string, value string) bool
	DelTrailer(name string) (deleted bool)

	// Unsafe
	UnsafeMake(size int) []byte
	UnsafeVersion() []byte
	UnsafeScheme() []byte
	UnsafeMethod() []byte
	UnsafeAuthority() []byte
	UnsafeHostname() []byte
	UnsafeColonPort() []byte
	UnsafeURI() []byte
	UnsafePath() []byte
	UnsafeEncodedPath() []byte
	UnsafeQueryString() []byte
	UnsafeQuery(name string) (value []byte, ok bool)
	UnsafeHeader(name string) (value []byte, ok bool)
	UnsafeCookie(name string) (value []byte, ok bool)
	UnsafeUserAgent() []byte
	UnsafeContentType() []byte
	UnsafeContent() []byte
	UnsafeForm(name string) (value []byte, ok bool)
	UnsafeTrailer(name string) (value []byte, ok bool)

	// Internal only
	arrayCopy(p []byte) bool
	getPathInfo() os.FileInfo
	unsafeAbsPath() []byte
	makeAbsPath()
	applyHeader(header *pair) bool
	delHost()
	delCriticalHeaders()
	delHopHeaders()
	walkHeaders(fn func(name []byte, value []byte) bool, withConnection bool) bool
	walkTrailers(fn func(name []byte, value []byte) bool, withConnection bool) bool
	recvContent(retain bool) any
	holdContent() any
	readContent() (p []byte, err error)
	hasTrailers() bool
	delHopTrailers()
	applyTrailer(trailer *pair) bool
	getSaveContentFilesDir() string
	hookReviser(reviser Reviser)
	unsafeVariable(index int16) []byte
}

// Response is the server-side HTTP response and is the interface for *http[1-3]Response.
type Response interface {
	Request() Request

	SetStatus(status int16) error
	Status() int16

	AddContentType(contentType string) bool
	SetLastModified(lastModified int64) bool
	SetETag(etag string) bool
	SetETagBytes(etag []byte) bool
	SetAcceptBytesRange()
	AddHTTPSRedirection(authority string) bool
	AddHostnameRedirection(hostname string) bool
	AddDirectoryRedirection() bool

	AddCookie(cookie *Cookie) bool

	AddHeader(name string, value string) bool
	AddHeaderBytes(name string, value []byte) bool
	AddHeaderByBytes(name []byte, value string) bool
	AddHeaderBytesByBytes(name []byte, value []byte) bool
	Header(name string) (value string, ok bool)
	DelHeader(name string) bool
	DelHeaderByBytes(name []byte) bool

	IsSent() bool
	SetMaxSendSeconds(seconds int64) // to defend against slowloris attack

	Send(content string) error
	SendBytes(content []byte) error
	SendJSON(content any) error
	SendFile(contentPath string) error

	SendBadRequest(content []byte) error
	SendForbidden(content []byte) error
	SendNotFound(content []byte) error
	SendMethodNotAllowed(allow string, content []byte) error
	SendInternalServerError(content []byte) error
	SendNotImplemented(content []byte) error
	SendBadGateway(content []byte) error
	SendGatewayTimeout(content []byte) error

	Push(chunk string) error
	PushBytes(chunk []byte) error
	PushFile(chunkPath string) error
	AddTrailer(name string, value string) bool

	// Internal only
	addHeader(name []byte, value []byte) bool
	header(name []byte) (value []byte, ok bool)
	delHeader(name []byte) bool
	makeETagFrom(modTime int64, fileSize int64) ([]byte, bool) // with ""
	setConnectionClose()
	sendBlob(content []byte) error
	sendFile(content *os.File, info os.FileInfo, shut bool) error // will close content after sent
	sendChain(chain Chain) error
	pushHeaders() error
	pushChain(chain Chain) error
	addTrailer(name []byte, value []byte) bool
	pass1xx(resp response) bool    // used by proxies
	copyHead(resp response) bool   // used by proxies
	pass(resp httpInMessage) error // used by proxies
	finishChunked() error
	post(content any, hasTrailers bool) error // used by proxies
	finalizeChunked() error
	hookReviser(reviser Reviser)
	unsafeMake(size int) []byte
}

// Socket is the server-side WebSocket and is the interface for *http[1-3]Socket.
type Socket interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
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
	rShell reflect.Value // the shell handler
	router Router
}

func (h *Handler_) UseRouter(shell any, router Router) {
	h.rShell = reflect.ValueOf(shell)
	h.router = router
}

func (h *Handler_) Dispatch(req Request, resp Response, notFound Handle) {
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

func (h *Handler_) IsProxy() bool { return false } // override this for proxy handlers
func (h *Handler_) IsCache() bool { return false } // override this for cache handlers

// Handle is a function which can handle http request and gives http response.
type Handle func(req Request, resp Response)

// Reviser component revises the outgoing response.
type Reviser interface {
	Component
	ider

	Rank() int8 // 0-31 (with 0-15 for user, 16-31 for fixed)

	BeforeRecv(req Request, resp Response) // for sized content

	BeforePull(req Request, resp Response) // for chunked content
	FinishPull(req Request, resp Response) // for chunked content

	Change(req Request, resp Response, chain Chain) Chain

	BeforeSend(req Request, resp Response) // for sized content

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
	app      *App      // associated app
	handlers []Handler // handlers in this rule. NOTICE: handlers are sub components of app, not rule
	revisers []Reviser // revisers in this rule. NOTICE: revisers are sub components of app, not rule
	socklets []Socklet // socklets in this rule. NOTICE: socklets are sub components of app, not rule
	// States
	general    bool     // general match?
	book       bool     // enable booking for this rule?
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

	// book
	r.ConfigureBool("book", &r.book, true)

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
	r.app.SubDone()
}

func (r *Rule) addHandler(handler Handler) {
	r.handlers = append(r.handlers, handler)
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
	// Hook revisers on request and response. When receiving or sending content, these revisers will be executed.
	for _, reviser := range r.revisers {
		req.hookReviser(reviser)
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

// Cookie is a cookie sent to client.
type Cookie struct {
	name     string
	value    string
	expires  time.Time
	maxAge   int64
	domain   string
	path     string
	sameSite string
	secure   bool
	httpOnly bool
	invalid  bool
	quote    bool // if true, quote value with ""
	aFrom    int8
	aEdge    int8
	ageBuf   [19]byte
}

func (c *Cookie) Set(name string, value string) bool {
	// cookie-name = 1*cookie-octet
	// cookie-octet = %x21 / %x23-2B / %x2D-3A / %x3C-5B / %x5D-7E
	if name == "" {
		c.invalid = true
		return false
	}
	for i := 0; i < len(name); i++ {
		if b := name[i]; httpKchar[b] == 0 {
			c.invalid = true
			return false
		}
	}
	c.name = name
	// cookie-value = *cookie-octet / ( DQUOTE *cookie-octet DQUOTE )
	for i := 0; i < len(value); i++ {
		b := value[i]
		if httpKchar[b] == 1 {
			continue
		}
		if b == ' ' || b == ',' {
			c.quote = true
			continue
		}
		c.invalid = true
		return false
	}
	c.value = value
	return true
}

func (c *Cookie) SetDomain(domain string) bool {
	// TODO: check domain
	c.domain = domain
	return true
}
func (c *Cookie) SetPath(path string) bool {
	// path-value = *av-octet
	// av-octet = %x20-3A / %x3C-7E
	for i := 0; i < len(path); i++ {
		if b := path[i]; b < 0x20 || b > 0x7E || b == 0x3B {
			c.invalid = true
			return false
		}
	}
	c.path = path
	return true
}
func (c *Cookie) SetExpires(expires time.Time) bool {
	if expires.Year() < 1601 {
		c.invalid = true
		return false
	}
	c.expires = expires
	return true
}
func (c *Cookie) SetMaxAge(maxAge int64) { c.maxAge = maxAge }
func (c *Cookie) SetSecure()             { c.secure = true }
func (c *Cookie) SetHttpOnly()           { c.httpOnly = true }
func (c *Cookie) SetSameSiteStrict()     { c.sameSite = "Strict" }
func (c *Cookie) SetSameSiteLax()        { c.sameSite = "Lax" }
func (c *Cookie) SetSameSiteNone()       { c.sameSite = "None" }

func (c *Cookie) size() int {
	// set-cookie: name=value; Expires=Sun, 06 Nov 1994 08:49:37 GMT; Max-Age=123; Domain=example.com; Path=/; Secure; HttpOnly; SameSite=Strict
	n := len(c.name) + 1 + len(c.value) // name=value
	if c.quote {
		n += 2 // ""
	}
	if !c.expires.IsZero() {
		n += len("; Expires=Sun, 06 Nov 1994 08:49:37 GMT")
	}
	if c.maxAge > 0 {
		from, edge := i64ToDec(c.maxAge, c.ageBuf[:])
		c.aFrom, c.aEdge = int8(from), int8(edge)
		n += len("; Max-Age=") + (edge - from)
	} else if c.maxAge < 0 {
		c.ageBuf[0] = '0'
		c.aFrom, c.aEdge = 0, 1
		n += len("; Max-Age=0")
	}
	if c.domain != "" {
		n += len("; Domain=") + len(c.domain)
	}
	if c.path != "" {
		n += len("; Path=") + len(c.path)
	}
	if c.secure {
		n += len("; Secure")
	}
	if c.httpOnly {
		n += len("; HttpOnly")
	}
	if c.sameSite != "" {
		n += len("; SameSite=") + len(c.sameSite)
	}
	return n
}
func (c *Cookie) writeTo(p []byte) int {
	i := copy(p, c.name)
	p[i] = '='
	i++
	if c.quote {
		p[i] = '"'
		i++
		i += copy(p[i:], c.value)
		p[i] = '"'
		i++
	} else {
		i += copy(p[i:], c.value)
	}
	if !c.expires.IsZero() {
		i += copy(p[i:], "; Expires=")
		i += clockWriteHTTPDate(c.expires, p[i:])
	}
	if c.maxAge != 0 {
		i += copy(p[i:], "; Max-Age=")
		i += copy(p[i:], c.ageBuf[c.aFrom:c.aEdge])
	}
	if c.domain != "" {
		i += copy(p[i:], "; Domain=")
		i += copy(p[i:], c.domain)
	}
	if c.path != "" {
		i += copy(p[i:], "; Path=")
		i += copy(p[i:], c.path)
	}
	if c.secure {
		i += copy(p[i:], "; Secure")
	}
	if c.httpOnly {
		i += copy(p[i:], "; HttpOnly")
	}
	if c.sameSite != "" {
		i += copy(p[i:], "; SameSite=")
		i += copy(p[i:], c.sameSite)
	}
	return i
}
