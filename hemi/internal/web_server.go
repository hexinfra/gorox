// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General Web server implementation for HTTP and HWEB.

package internal

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/risky"
)

// webServer is the interface for *http[x3]Server and *hweb[1-2]Server.
type webServer interface {
	Server
	streamHolder
	contentSaver

	MaxContentSize() int64
	RecvTimeout() time.Duration
	SendTimeout() time.Duration

	linkApps()
	findApp(hostname []byte) *App
}

// webServer_ is a mixin for http[x3]Server and hweb[1-2]Server.
type webServer_ struct {
	// Mixins
	Server_
	webAgent_
	streamHolder_
	contentSaver_ // so requests can save their large contents in local file system. if request is dispatched to app, we use app's contentSaver_.
	// Assocs
	gates      []webGate
	defaultApp *App // default app
	// States
	forApps      []string            // for what apps
	exactApps    []*hostnameTo[*App] // like: ("example.com")
	suffixApps   []*hostnameTo[*App] // like: ("*.example.com")
	prefixApps   []*hostnameTo[*App] // like: ("www.example.*")
	enableTCPTun bool                // allow CONNECT method?
	enableUDPTun bool                // allow upgrade: connect-udp?
}

func (s *webServer_) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)
}

func (s *webServer_) onConfigure(shell Component) {
	s.Server_.OnConfigure()
	s.webAgent_.onConfigure(shell, 120*time.Second, 120*time.Second)
	s.streamHolder_.onConfigure(shell, 0)
	s.contentSaver_.onConfigure(shell, TempDir()+"/web/servers/"+s.name)

	// forApps
	s.ConfigureStringList("forApps", &s.forApps, nil, []string{})
	// enableTCPTun
	s.ConfigureBool("enableTCPTun", &s.enableTCPTun, false)
	// enableUDPTun
	s.ConfigureBool("enableUDPTun", &s.enableUDPTun, false)
}
func (s *webServer_) onPrepare(shell Component) {
	s.Server_.OnPrepare()
	s.streamHolder_.onPrepare(shell)
	s.contentSaver_.onPrepare(shell, 0755)
}

func (s *webServer_) linkApps() {
	for _, appName := range s.forApps {
		app := s.stage.App(appName)
		if app == nil {
			continue
		}
		if s.tlsConfig != nil {
			if app.tlsCertificate == "" || app.tlsPrivateKey == "" {
				UseExitln("apps that bound to tls server must have certificates and private keys")
			}
			certificate, err := tls.LoadX509KeyPair(app.tlsCertificate, app.tlsPrivateKey)
			if err != nil {
				UseExitln(err.Error())
			}
			if IsDebug(1) {
				Debugf("adding certificate to %s\n", s.ColonPort())
			}
			s.tlsConfig.Certificates = append(s.tlsConfig.Certificates, certificate)
		}
		app.linkServer(s.shell.(webServer))
		if app.isDefault {
			s.defaultApp = app
		}
		// TODO: use hash table?
		for _, hostname := range app.exactHostnames {
			s.exactApps = append(s.exactApps, &hostnameTo[*App]{hostname, app})
		}
		// TODO: use radix trie?
		for _, hostname := range app.suffixHostnames {
			s.suffixApps = append(s.suffixApps, &hostnameTo[*App]{hostname, app})
		}
		// TODO: use radix trie?
		for _, hostname := range app.prefixHostnames {
			s.prefixApps = append(s.prefixApps, &hostnameTo[*App]{hostname, app})
		}
	}
}
func (s *webServer_) findApp(hostname []byte) *App {
	// TODO: use hash table?
	for _, exactMap := range s.exactApps {
		if bytes.Equal(hostname, exactMap.hostname) {
			return exactMap.target
		}
	}
	// TODO: use radix trie?
	for _, suffixMap := range s.suffixApps {
		if bytes.HasSuffix(hostname, suffixMap.hostname) {
			return suffixMap.target
		}
	}
	// TODO: use radix trie?
	for _, prefixMap := range s.prefixApps {
		if bytes.HasPrefix(hostname, prefixMap.hostname) {
			return prefixMap.target
		}
	}
	return s.defaultApp
}

// webGate is the interface for *http[x3]Gate and *hweb[1-2]Gate.
type webGate interface {
	Gate
	onConnectionClosed()
}

// webGate_ is the mixin for http[x3]Gate and hweb[1-2]Gate.
type webGate_ struct {
	// Mixins
	Gate_
}

func (g *webGate_) onConnectionClosed() {
	g.DecConns()
	g.SubDone()
}

// serverConn is the interface for *http[1-3]Conn and *hweb[1-2]Conn.
type serverConn interface {
	serve() // goroutine
	getServer() webServer
	isBroken() bool
	markBroken()
	makeTempName(p []byte, unixTime int64) (from int, edge int) // small enough to be placed in buffer256() of stream
}

// serverConn_ is the mixin for http[1-3]Conn and hweb[1-2]Conn.
type serverConn_ struct {
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id     int64     // the conn id
	server webServer // the server to which the conn belongs
	gate   webGate   // the gate to which the conn belongs
	// Conn states (zeros)
	lastRead    time.Time    // deadline of last read operation
	lastWrite   time.Time    // deadline of last write operation
	counter     atomic.Int64 // together with id, used to generate a random number as uploaded file's path
	usedStreams atomic.Int32 // num of streams served
	broken      atomic.Bool  // is conn broken?
}

func (c *serverConn_) onGet(id int64, server webServer, gate webGate) {
	c.id = id
	c.server = server
	c.gate = gate
}
func (c *serverConn_) onPut() {
	c.server = nil
	c.gate = nil
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
	c.counter.Store(0)
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *serverConn_) getServer() webServer { return c.server }
func (c *serverConn_) getGate() webGate     { return c.gate }

func (c *serverConn_) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return makeTempName(p, int64(c.server.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *serverConn_) isBroken() bool { return c.broken.Load() }
func (c *serverConn_) markBroken()    { c.broken.Store(true) }

// Request is the interface for *http[1-3]Request and *hweb[1-2]Request.
type Request interface {
	PeerAddr() net.Addr
	App() *App

	VersionCode() uint8
	IsHTTP1_0() bool
	IsHTTP1_1() bool
	IsHTTP1() bool
	IsHTTP2() bool
	IsHTTP3() bool
	Version() string // HTTP/1.0, HTTP/1.1, HTTP/2, HTTP/3

	SchemeCode() uint8 // SchemeHTTP, SchemeHTTPS
	IsHTTP() bool
	IsHTTPS() bool
	Scheme() string // http, https

	MethodCode() uint32
	Method() string // GET, POST, ...
	IsGET() bool
	IsPOST() bool
	IsPUT() bool
	IsDELETE() bool

	IsAbsoluteForm() bool    // TODO: what about HTTP/2 and HTTP/3?
	IsAsteriskOptions() bool // OPTIONS *

	Authority() string // hostname[:port]
	Hostname() string  // hostname
	ColonPort() string // :port

	URI() string         // /encodedPath?queryString
	Path() string        // /path
	EncodedPath() string // /encodedPath
	QueryString() string // including '?' if query string exists, otherwise empty

	AddQuery(name string, value string) bool
	HasQueries() bool
	AllQueries() (queries [][2]string)
	Q(name string) string
	Qstr(name string, defaultValue string) string
	Qint(name string, defaultValue int) int
	Query(name string) (value string, ok bool)
	Queries(name string) (values []string, ok bool)
	HasQuery(name string) bool
	DelQuery(name string) (deleted bool)

	AddHeader(name string, value string) bool
	HasHeaders() bool
	AllHeaders() (headers [][2]string)
	H(name string) string
	Hstr(name string, defaultValue string) string
	Hint(name string, defaultValue int) int
	Header(name string) (value string, ok bool)
	Headers(name string) (values []string, ok bool)
	HasHeader(name string) bool
	DelHeader(name string) (deleted bool)

	UserAgent() string
	ContentType() string
	ContentSize() int64
	AcceptTrailers() bool

	TestConditions(modTime int64, etag []byte, asOrigin bool) (status int16, pass bool) // to test preconditons intentionally
	TestIfRanges(modTime int64, etag []byte, asOrigin bool) (pass bool)                 // to test preconditons intentionally

	AddCookie(name string, value string) bool
	HasCookies() bool
	AllCookies() (cookies [][2]string)
	C(name string) string
	Cstr(name string, defaultValue string) string
	Cint(name string, defaultValue int) int
	Cookie(name string) (value string, ok bool)
	Cookies(name string) (values []string, ok bool)
	HasCookie(name string) bool
	DelCookie(name string) (deleted bool)

	SetRecvTimeout(timeout time.Duration) // to defend against slowloris attack

	HasContent() bool // true if content exists
	IsUnsized() bool  // true if content exists and is not sized
	Content() string

	AddForm(name string, value string) bool
	HasForms() bool
	AllForms() (forms [][2]string)
	F(name string) string
	Fstr(name string, defaultValue string) string
	Fint(name string, defaultValue int) int
	Form(name string) (value string, ok bool)
	Forms(name string) (values []string, ok bool)
	HasForm(name string) bool

	HasUploads() bool
	AllUploads() (uploads []*Upload)
	U(name string) *Upload
	Upload(name string) (upload *Upload, ok bool)
	Uploads(name string) (uploads []*Upload, ok bool)
	HasUpload(name string) bool

	AddTrailer(name string, value string) bool
	HasTrailers() bool
	AllTrailers() (trailers [][2]string)
	T(name string) string
	Tstr(name string, defaultValue string) string
	Tint(name string, defaultValue int) int
	Trailer(name string) (value string, ok bool)
	Trailers(name string) (values []string, ok bool)
	HasTrailer(name string) bool
	DelTrailer(name string) (deleted bool)

	// Unsafe
	UnsafeMake(size int) []byte
	UnsafeVersion() []byte
	UnsafeScheme() []byte
	UnsafeMethod() []byte
	UnsafeAuthority() []byte // hostname[:port]
	UnsafeHostname() []byte  // hostname
	UnsafeColonPort() []byte // :port
	UnsafeURI() []byte
	UnsafePath() []byte
	UnsafeEncodedPath() []byte
	UnsafeQueryString() []byte // including '?' if query string exists, otherwise empty
	UnsafeQuery(name string) (value []byte, ok bool)
	UnsafeHeader(name string) (value []byte, ok bool)
	UnsafeCookie(name string) (value []byte, ok bool)
	UnsafeUserAgent() []byte
	UnsafeContentLength() []byte
	UnsafeContentType() []byte
	UnsafeContent() []byte
	UnsafeForm(name string) (value []byte, ok bool)
	UnsafeTrailer(name string) (value []byte, ok bool)

	// Internal only
	getPathInfo() os.FileInfo
	unsafeAbsPath() []byte
	makeAbsPath()
	delHopHeaders()
	forCookies(callback func(cookie *pair, name []byte, value []byte) bool) bool
	forHeaders(callback func(header *pair, name []byte, value []byte) bool) bool
	getRanges() []rang
	unsetHost()
	takeContent() any
	readContent() (p []byte, err error)
	applyTrailer(index uint8) bool
	delHopTrailers()
	forTrailers(callback func(trailer *pair, name []byte, value []byte) bool) bool
	arrayCopy(p []byte) bool
	saveContentFilesDir() string
	hookReviser(reviser Reviser)
	unsafeVariable(index int16) []byte
}

// serverRequest_ is the mixin for http[1-3]Request and hweb[1-2]Request.
type serverRequest_ struct { // incoming. needs parsing
	// Mixins
	webIn_ // incoming web message
	// Stream states (stocks)
	stockUploads [2]Upload // for r.uploads. 96B
	// Stream states (controlled)
	ranges [2]rang // parsed range fields. at most two range fields are allowed. controlled by r.nRanges
	// Stream states (non-zeros)
	uploads []Upload // decoded uploads -> r.array (for metadata) and temp files in local file system. [<r.stockUploads>/(make=16/128)]
	// Stream states (zeros)
	path           []byte      // decoded path. only a reference. refers to r.array or region if rewrited, so can't be a span
	absPath        []byte      // app.webRoot + r.UnsafePath(). if app.webRoot is not set then this is nil. set when dispatching to handlets. only a reference
	pathInfo       os.FileInfo // cached result of os.Stat(r.absPath) if r.absPath is not nil
	app            *App        // target app of this request. set before processing stream
	formWindow     []byte      // a window used when reading and parsing content as multipart/form-data. [<none>/r.contentText/4K/16K]
	serverRequest0             // all values must be zero by default in this struct!
}
type serverRequest0 struct { // for fast reset, entirely
	gotInput        bool     // got some input from client? for request timeout handling
	targetForm      int8     // http request-target form. see httpTargetXXX
	asteriskOptions bool     // OPTIONS *?
	schemeCode      uint8    // SchemeHTTP, SchemeHTTPS
	methodCode      uint32   // known method code. 0: unknown method
	method          span     // raw method -> r.input
	authority       span     // raw hostname[:port] -> r.input
	hostname        span     // raw hostname (without :port) -> r.input
	colonPort       span     // raw colon port (:port, with ':') -> r.input
	uri             span     // raw uri (raw path & raw query string) -> r.input
	encodedPath     span     // raw path -> r.input
	queryString     span     // raw query string (with '?') -> r.input
	boundary        span     // boundary parameter of "multipart/form-data" if exists -> r.input
	queries         zone     // decoded queries -> r.array
	cookies         zone     // cookies ->r.input. temporarily used when checking cookie headers, set after cookie header is parsed
	forms           zone     // decoded forms -> r.array
	ifMatch         int8     // -1: if-match *, 0: no if-match field, >0: number of if-match: 1#entity-tag
	ifNoneMatch     int8     // -1: if-none-match *, 0: no if-none-match field, >0: number of if-none-match: 1#entity-tag
	nRanges         int8     // num of ranges
	expectContinue  bool     // expect: 100-continue?
	acceptTrailers  bool     // does client accept trailers? i.e. te: trailers, gzip
	pathInfoGot     bool     // is r.pathInfo got?
	_               [4]byte  // padding
	indexes         struct { // indexes of some selected singleton headers, for fast accessing
		authorization      uint8 // authorization header ->r.input
		host               uint8 // host header ->r.input
		ifModifiedSince    uint8 // if-modified-since header ->r.input
		ifRange            uint8 // if-range header ->r.input
		ifUnmodifiedSince  uint8 // if-unmodified-since header ->r.input
		proxyAuthorization uint8 // proxy-authorization header ->r.input
		userAgent          uint8 // user-agent header ->r.input
		_                  byte  // padding
	}
	zones struct { // zones of some selected headers, for fast accessing
		acceptLanguage zone
		expect         zone
		forwarded      zone
		ifMatch        zone // the zone of if-match in r.primes. may be not continuous
		ifNoneMatch    zone // the zone of if-none-match in r.primes. may be not continuous
		xForwardedFor  zone
		_              [4]byte // padding
	}
	unixTimes struct { // parsed unixTimes
		ifModifiedSince   int64 // parsed unix time of if-modified-since
		ifRange           int64 // parsed unix time of if-range if is http-date format
		ifUnmodifiedSince int64 // parsed unix time of if-unmodified-since
	}
	cacheControl struct { // the cache-control info
		noCache      bool  // no-cache directive in cache-control
		noStore      bool  // no-store directive in cache-control
		noTransform  bool  // no-transform directive in cache-control
		onlyIfCached bool  // only-if-cached directive in cache-control
		maxAge       int32 // max-age directive in cache-control
		maxStale     int32 // max-stale directive in cache-control
		minFresh     int32 // min-fresh directive in cache-control
	}
	revisers     [32]uint8 // reviser ids which will apply on this request. indexed by reviser order
	_            [2]byte   // padding
	formReceived bool      // if content is a form, is it received?
	formKind     int8      // deducted type of form. 0:not form. see formXXX
	formEdge     int32     // edge position of the filled content in r.formWindow
	pFieldName   span      // field name. used during receiving and parsing multipart form in case of sliding r.formWindow
	consumedSize int64     // bytes of consumed content when consuming received tempFile. used by, for example, _recvMultipartForm.
}

func (r *serverRequest_) onUse(versionCode uint8) { // for non-zeros
	r.webIn_.onUse(versionCode, false) // asResponse = false

	r.uploads = r.stockUploads[0:0:cap(r.stockUploads)] // use append()
}
func (r *serverRequest_) onEnd() { // for zeros
	for _, upload := range r.uploads {
		if upload.isMoved() {
			continue
		}
		var path string
		if upload.metaSet() {
			path = upload.Path()
		} else {
			path = risky.WeakString(r.array[upload.pathFrom : upload.pathFrom+int32(upload.pathSize)])
		}
		if err := os.Remove(path); err != nil {
			r.app.Logf("failed to remove uploaded file: %s, error: %s\n", path, err.Error())
		}
	}
	r.uploads = nil

	r.path = nil
	r.absPath = nil
	r.pathInfo = nil
	r.app = nil
	r.formWindow = nil // if r.formWindow is fetched from pool, it's put into pool on return. so just set as nil
	r.serverRequest0 = serverRequest0{}

	r.webIn_.onEnd()
}

func (r *serverRequest_) App() *App { return r.app }

func (r *serverRequest_) SchemeCode() uint8    { return r.schemeCode }
func (r *serverRequest_) Scheme() string       { return httpSchemeStrings[r.schemeCode] }
func (r *serverRequest_) UnsafeScheme() []byte { return httpSchemeByteses[r.schemeCode] }
func (r *serverRequest_) IsHTTP() bool         { return r.schemeCode == SchemeHTTP }
func (r *serverRequest_) IsHTTPS() bool        { return r.schemeCode == SchemeHTTPS }

func (r *serverRequest_) MethodCode() uint32   { return r.methodCode }
func (r *serverRequest_) Method() string       { return string(r.UnsafeMethod()) }
func (r *serverRequest_) UnsafeMethod() []byte { return r.input[r.method.from:r.method.edge] }
func (r *serverRequest_) IsGET() bool          { return r.methodCode == MethodGET }
func (r *serverRequest_) IsPOST() bool         { return r.methodCode == MethodPOST }
func (r *serverRequest_) IsPUT() bool          { return r.methodCode == MethodPUT }
func (r *serverRequest_) IsDELETE() bool       { return r.methodCode == MethodDELETE }
func (r *serverRequest_) recognizeMethod(method []byte, hash uint16) {
	if m := httpMethodTable[httpMethodFind(hash)]; m.hash == hash && bytes.Equal(httpMethodBytes[m.from:m.edge], method) {
		r.methodCode = m.code
	}
}

func (r *serverRequest_) IsAsteriskOptions() bool { return r.asteriskOptions }
func (r *serverRequest_) IsAbsoluteForm() bool    { return r.targetForm == httpTargetAbsolute }

func (r *serverRequest_) Authority() string       { return string(r.UnsafeAuthority()) }
func (r *serverRequest_) UnsafeAuthority() []byte { return r.input[r.authority.from:r.authority.edge] }
func (r *serverRequest_) Hostname() string        { return string(r.UnsafeHostname()) }
func (r *serverRequest_) UnsafeHostname() []byte  { return r.input[r.hostname.from:r.hostname.edge] }
func (r *serverRequest_) ColonPort() string {
	if r.colonPort.notEmpty() {
		return string(r.input[r.colonPort.from:r.colonPort.edge])
	}
	if r.schemeCode == SchemeHTTPS {
		return stringColonPort443
	} else {
		return stringColonPort80
	}
}
func (r *serverRequest_) UnsafeColonPort() []byte {
	if r.colonPort.notEmpty() {
		return r.input[r.colonPort.from:r.colonPort.edge]
	}
	if r.schemeCode == SchemeHTTPS {
		return bytesColonPort443
	} else {
		return bytesColonPort80
	}
}

func (r *serverRequest_) URI() string {
	if r.uri.notEmpty() {
		return string(r.input[r.uri.from:r.uri.edge])
	} else { // use "/"
		return stringSlash
	}
}
func (r *serverRequest_) UnsafeURI() []byte {
	if r.uri.notEmpty() {
		return r.input[r.uri.from:r.uri.edge]
	} else { // use "/"
		return bytesSlash
	}
}
func (r *serverRequest_) EncodedPath() string {
	if r.encodedPath.notEmpty() {
		return string(r.input[r.encodedPath.from:r.encodedPath.edge])
	} else { // use "/"
		return stringSlash
	}
}
func (r *serverRequest_) UnsafeEncodedPath() []byte {
	if r.encodedPath.notEmpty() {
		return r.input[r.encodedPath.from:r.encodedPath.edge]
	} else { // use "/"
		return bytesSlash
	}
}
func (r *serverRequest_) Path() string {
	if len(r.path) != 0 {
		return string(r.path)
	} else { // use "/"
		return stringSlash
	}
}
func (r *serverRequest_) UnsafePath() []byte {
	if len(r.path) != 0 {
		return r.path
	} else { // use "/"
		return bytesSlash
	}
}
func (r *serverRequest_) cleanPath() {
	nPath := len(r.path)
	if nPath <= 1 {
		// Must be '/'.
		return
	}
	slash := r.path[nPath-1] == '/'
	pOrig, pReal := 1, 1
	for pOrig < nPath {
		if b := r.path[pOrig]; b == '/' {
			pOrig++
		} else if b == '.' && (pOrig+1 == nPath || r.path[pOrig+1] == '/') {
			pOrig++
		} else if b == '.' && r.path[pOrig+1] == '.' && (pOrig+2 == nPath || r.path[pOrig+2] == '/') {
			pOrig += 2
			if pReal > 1 {
				pReal--
				for pReal > 1 && r.path[pReal] != '/' {
					pReal--
				}
			}
		} else {
			if pReal != 1 {
				r.path[pReal] = '/'
				pReal++
			}
			for pOrig < nPath && r.path[pOrig] != '/' {
				r.path[pReal] = r.path[pOrig]
				pReal++
				pOrig++
			}
		}
	}
	if pReal != nPath {
		if slash && pReal > 1 {
			r.path[pReal] = '/'
			pReal++
		}
		r.path = r.path[:pReal]
	}
}
func (r *serverRequest_) unsafeAbsPath() []byte { return r.absPath }
func (r *serverRequest_) makeAbsPath() {
	if r.app.webRoot == "" { // if app's webRoot is empty, r.absPath is not used either. so it's safe to do nothing
		return
	}
	webRoot := r.app.webRoot
	r.absPath = r.UnsafeMake(len(webRoot) + len(r.UnsafePath()))
	n := copy(r.absPath, webRoot)
	copy(r.absPath[n:], r.UnsafePath())
}
func (r *serverRequest_) getPathInfo() os.FileInfo {
	if !r.pathInfoGot {
		r.pathInfoGot = true
		if pathInfo, err := os.Stat(risky.WeakString(r.absPath)); err == nil {
			r.pathInfo = pathInfo
		}
	}
	return r.pathInfo
}
func (r *serverRequest_) QueryString() string { return string(r.UnsafeQueryString()) }
func (r *serverRequest_) UnsafeQueryString() []byte {
	return r.input[r.queryString.from:r.queryString.edge]
}

func (r *serverRequest_) addQuery(query *pair) bool { // as prime
	if edge, ok := r._addPrime(query); ok {
		r.queries.edge = edge
		return true
	}
	r.headResult, r.failReason = StatusURITooLong, "too many queries"
	return false
}
func (r *serverRequest_) AddQuery(name string, value string) bool { // as extra
	return r.addExtra(name, value, 0, kindQuery)
}
func (r *serverRequest_) HasQueries() bool                  { return r.hasPairs(r.queries, kindQuery) }
func (r *serverRequest_) AllQueries() (queries [][2]string) { return r.allPairs(r.queries, kindQuery) }
func (r *serverRequest_) Q(name string) string {
	value, _ := r.Query(name)
	return value
}
func (r *serverRequest_) Qstr(name string, defaultValue string) string {
	if value, ok := r.Query(name); ok {
		return value
	}
	return defaultValue
}
func (r *serverRequest_) Qint(name string, defaultValue int) int {
	if value, ok := r.Query(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *serverRequest_) Query(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.queries, kindQuery)
	return string(v), ok
}
func (r *serverRequest_) UnsafeQuery(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.queries, kindQuery)
}
func (r *serverRequest_) Queries(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.queries, kindQuery)
}
func (r *serverRequest_) HasQuery(name string) bool {
	_, ok := r.getPair(name, 0, r.queries, kindQuery)
	return ok
}
func (r *serverRequest_) DelQuery(name string) (deleted bool) {
	return r.delPair(name, 0, r.queries, kindQuery)
}

func (r *serverRequest_) examineHead() bool {
	for i := r.headers.from; i < r.headers.edge; i++ {
		if !r.applyHeader(i) {
			// r.headResult is set.
			return false
		}
	}
	if r.cookies.notEmpty() { // in HTTP/2 and HTTP/3, there can be multiple cookie fields.
		cookies := r.cookies // make a copy. r.cookies is changed as cookie pairs below
		r.cookies.from = uint8(len(r.primes))
		for i := cookies.from; i < cookies.edge; i++ {
			cookie := &r.primes[i]
			if cookie.hash != hashCookie || !cookie.nameEqualBytes(r.input, bytesCookie) { // cookies may not be consecutive
				continue
			}
			if !r.parseCookie(cookie.value) { // r.cookies.edge is set in r.addCookie().
				return false
			}
		}
	}
	if IsDebug(2) {
		Debugln("======primes======")
		for i := 0; i < len(r.primes); i++ {
			prime := &r.primes[i]
			prime.show(r._placeOf(prime))
		}
		Debugln("======extras======")
		for i := 0; i < len(r.extras); i++ {
			extra := &r.extras[i]
			extra.show(r._placeOf(extra))
		}
	}

	// RFC 7230 (section 3.2.2. Field Order): A server MUST NOT
	// apply a request to the target resource until the entire request
	// header section is received, since later header fields might include
	// conditionals, authentication credentials, or deliberately misleading
	// duplicate header fields that would impact request processing.

	// Basic checks against versions
	switch r.versionCode {
	case Version1_0:
		if r.keepAlive == -1 { // no connection header
			r.keepAlive = 0 // default is close for HTTP/1.0
		}
	case Version1_1:
		if r.indexes.host == 0 {
			// RFC 7230 (section 5.4):
			// A client MUST send a Host header field in all HTTP/1.1 request messages.
			r.headResult, r.failReason = StatusBadRequest, "MUST send a Host header field in all HTTP/1.1 request messages"
			return false
		}
		if r.keepAlive == -1 { // no connection header
			r.keepAlive = 1 // default is keep-alive for HTTP/1.1
		}
	default: // HTTP/2 and HTTP/3
		// Add here
	}

	if !r.determineContentMode() {
		// r.headResult is set.
		return false
	}
	if r.contentSize > r.maxContentSize {
		r.headResult, r.failReason = StatusContentTooLarge, "content size exceeds server's limit"
		return false
	}

	if r.upgradeSocket && (r.methodCode != MethodGET || r.versionCode == Version1_0 || r.contentSize != -1) {
		// RFC 6455 (section 4.1):
		// The method of the request MUST be GET, and the HTTP version MUST be at least 1.1.
		r.headResult, r.failReason = StatusMethodNotAllowed, "websocket only supports GET method and HTTP version >= 1.1, without content"
		return false
	}
	if r.methodCode&(MethodCONNECT|MethodOPTIONS|MethodTRACE) != 0 {
		// RFC 7232 (section 5):
		// Likewise, a server
		// MUST ignore the conditional request header fields defined by this
		// specification when received with a request method that does not
		// involve the selection or modification of a selected representation,
		// such as CONNECT, OPTIONS, or TRACE.
		if r.ifMatch != 0 {
			r.delHeader(bytesIfMatch, hashIfMatch)
			r.ifMatch = 0
		}
		if r.ifNoneMatch != 0 {
			r.delHeader(bytesIfNoneMatch, hashIfNoneMatch)
			r.ifNoneMatch = 0
		}
		if r.indexes.ifModifiedSince != 0 {
			r._delPrime(r.indexes.ifModifiedSince)
			r.indexes.ifModifiedSince = 0
		}
		if r.indexes.ifUnmodifiedSince != 0 {
			r._delPrime(r.indexes.ifUnmodifiedSince)
			r.indexes.ifUnmodifiedSince = 0
		}
		if r.indexes.ifRange != 0 {
			r._delPrime(r.indexes.ifRange)
			r.indexes.ifRange = 0
		}
	} else {
		// RFC 9110 (section 13.1.3):
		// A recipient MUST ignore the If-Modified-Since header field if the
		// received field value is not a valid HTTP-date, the field value has
		// more than one member, or if the request method is neither GET nor HEAD.
		if r.indexes.ifModifiedSince != 0 && r.methodCode&(MethodGET|MethodHEAD) == 0 {
			r._delPrime(r.indexes.ifModifiedSince) // we delete it.
			r.indexes.ifModifiedSince = 0
		}
		// A server MUST ignore an If-Range header field received in a request that does not contain a Range header field.
		if r.indexes.ifRange != 0 && r.nRanges == 0 {
			r._delPrime(r.indexes.ifRange) // we delete it.
			r.indexes.ifRange = 0
		}
	}
	if r.contentSize == -1 { // no content
		if r.expectContinue { // expect is used to send large content.
			r.headResult, r.failReason = StatusBadRequest, "cannot use expect header without content"
			return false
		}
		if r.methodCode&(MethodPOST|MethodPUT) != 0 {
			r.headResult, r.failReason = StatusLengthRequired, "POST and PUT must contain a content"
			return false
		}
	} else { // content exists (sized or unsized)
		// Content is not allowed in some methods, according to RFC 7231.
		if r.methodCode&(MethodCONNECT|MethodTRACE) != 0 {
			r.headResult, r.failReason = StatusBadRequest, "content is not allowed in CONNECT and TRACE method"
			return false
		}
		if r.nContentCodings > 0 { // have content-encoding
			if r.nContentCodings > 1 || r.contentCodings[0] != httpCodingGzip {
				r.headResult, r.failReason = StatusUnsupportedMediaType, "currently only gzip content coding is supported in request"
				return false
			}
		}
		if r.iContentType == 0 { // no content-type
			if r.methodCode == MethodOPTIONS {
				// RFC 7231 (section 4.3.7):
				// A client that generates an OPTIONS request containing a payload body
				// MUST send a valid Content-Type header field describing the
				// representation media type.
				r.headResult, r.failReason = StatusBadRequest, "OPTIONS with content but without a content-type"
				return false
			}
		} else { // content-type exists
			header := &r.primes[r.iContentType]
			contentType := header.dataAt(r.input)
			bytesToLower(contentType)
			if bytes.Equal(contentType, bytesURLEncodedForm) {
				r.formKind = httpFormURLEncoded
			} else if bytes.Equal(contentType, bytesMultipartForm) { // multipart/form-data; boundary=xxxxxx
				for i := header.params.from; i < header.params.edge; i++ {
					param := &r.extras[i]
					if param.hash != hashBoundary || !param.nameEqualBytes(r.input, bytesBoundary) {
						continue
					}
					if boundary := param.value; boundary.notEmpty() && boundary.size() <= 70 && r.input[boundary.edge-1] != ' ' {
						// boundary := 0*69<bchars> bcharsnospace
						// bchars := bcharsnospace / " "
						// bcharsnospace := DIGIT / ALPHA / "'" / "(" / ")" / "+" / "_" / "," / "-" / "." / "/" / ":" / "=" / "?"
						r.boundary = boundary
						r.formKind = httpFormMultipart
						break
					}
				}
				if r.formKind != httpFormMultipart {
					r.headResult, r.failReason = StatusBadRequest, "bad boundary"
					return false
				}
			}
			if r.formKind != httpFormNotForm && r.nContentCodings > 0 {
				r.headResult, r.failReason = StatusUnsupportedMediaType, "a form with content coding is not supported yet"
				return false
			}
		}
	}

	return true
}
func (r *serverRequest_) applyHeader(index uint8) bool {
	header := &r.primes[index]
	name := header.nameAt(r.input)
	if sh := &serverRequestSingletonHeaderTable[serverRequestSingletonHeaderFind(header.hash)]; sh.hash == header.hash && bytes.Equal(sh.name, name) {
		header.setSingleton()
		if !sh.parse { // unnecessary to parse
			header.setParsed()
			header.dataEdge = header.value.edge
		} else if !r._parseField(header, &sh.desc, r.input, true) {
			r.headResult = StatusBadRequest
			return false
		}
		if !sh.check(r, header, index) {
			// r.headResult is set.
			return false
		}
	} else if mh := &serverRequestImportantHeaderTable[serverRequestImportantHeaderFind(header.hash)]; mh.hash == header.hash && bytes.Equal(mh.name, name) {
		extraFrom := uint8(len(r.extras))
		if !r._splitField(header, &mh.desc, r.input) {
			r.headResult = StatusBadRequest
			return false
		}
		if header.isCommaValue() { // has sub headers, check them
			if !mh.check(r, r.extras, extraFrom, uint8(len(r.extras))) {
				// r.headResult is set.
				return false
			}
		} else if !mh.check(r, r.primes, index, index+1) { // no sub headers. check it
			// r.headResult is set.
			return false
		}
	} else {
		// All other headers are treated as list-based headers.
	}
	return true
}

var ( // perfect hash table for singleton request headers
	serverRequestSingletonHeaderTable = [12]struct {
		parse bool // need general parse or not
		desc       // allowQuote, allowEmpty, allowParam, hasComment
		check func(*serverRequest_, *pair, uint8) bool
	}{ // authorization content-length content-type cookie date host if-modified-since if-range if-unmodified-since proxy-authorization range user-agent
		0:  {false, desc{hashIfUnmodifiedSince, false, false, false, false, bytesIfUnmodifiedSince}, (*serverRequest_).checkIfUnmodifiedSince},
		1:  {false, desc{hashUserAgent, false, false, false, true, bytesUserAgent}, (*serverRequest_).checkUserAgent},
		2:  {false, desc{hashContentLength, false, false, false, false, bytesContentLength}, (*serverRequest_).checkContentLength},
		3:  {false, desc{hashRange, false, false, false, false, bytesRange}, (*serverRequest_).checkRange},
		4:  {false, desc{hashDate, false, false, false, false, bytesDate}, (*serverRequest_).checkDate},
		5:  {false, desc{hashHost, false, false, false, false, bytesHost}, (*serverRequest_).checkHost},
		6:  {false, desc{hashCookie, false, false, false, false, bytesCookie}, (*serverRequest_).checkCookie}, // `a=b; c=d; e=f` is cookie list, not parameters
		7:  {true, desc{hashContentType, false, false, true, false, bytesContentType}, (*serverRequest_).checkContentType},
		8:  {false, desc{hashIfRange, false, false, false, false, bytesIfRange}, (*serverRequest_).checkIfRange},
		9:  {false, desc{hashIfModifiedSince, false, false, false, false, bytesIfModifiedSince}, (*serverRequest_).checkIfModifiedSince},
		10: {false, desc{hashAuthorization, false, false, false, false, bytesAuthorization}, (*serverRequest_).checkAuthorization},
		11: {false, desc{hashProxyAuthorization, false, false, false, false, bytesProxyAuthorization}, (*serverRequest_).checkProxyAuthorization},
	}
	serverRequestSingletonHeaderFind = func(hash uint16) int { return (612750 / int(hash)) % 12 }
)

func (r *serverRequest_) checkAuthorization(header *pair, index uint8) bool { // Authorization = auth-scheme [ 1*SP ( token68 / #auth-param ) ]
	// auth-scheme = token
	// token68     = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
	// auth-param  = token BWS "=" BWS ( token / quoted-string )
	// TODO
	if r.indexes.authorization == 0 {
		r.indexes.authorization = index
		return true
	} else {
		r.headResult, r.failReason = StatusBadRequest, "duplicated authorization"
		return false
	}
}
func (r *serverRequest_) checkCookie(header *pair, index uint8) bool { // Cookie = cookie-string
	if header.value.isEmpty() {
		r.headResult, r.failReason = StatusBadRequest, "empty cookie"
		return false
	}
	if index == 255 {
		r.headResult, r.failReason = StatusBadRequest, "too many pairs"
		return false
	}
	// HTTP/2 and HTTP/3 allows multiple cookie headers, so we have to mark all the cookie headers.
	if r.cookies.isEmpty() {
		r.cookies.from = index
	}
	// And we can't inject cookies into headers zone while receiving headers, this will break the continuous nature of headers zone.
	r.cookies.edge = index + 1 // so we postpone cookie parsing after the request head is entirely received. only mark the edge
	return true
}
func (r *serverRequest_) checkHost(header *pair, index uint8) bool { // Host = host [ ":" port ]
	// RFC 7230 (section 5.4): A server MUST respond with a 400 (Bad Request) status code to any
	// HTTP/1.1 request message that lacks a Host header field and to any request message that
	// contains more than one Host header field or a Host header field with an invalid field-value.
	if r.indexes.host != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicate host header"
		return false
	}
	host := header.value
	if host.notEmpty() {
		// RFC 7230 (section 2.7.3.  http and https URI Normalization and Comparison):
		// The scheme and host are case-insensitive and normally provided in lowercase;
		// all other components are compared in a case-sensitive manner.
		bytesToLower(r.input[host.from:host.edge])
		if !r.parseAuthority(host.from, host.edge, r.authority.isEmpty()) {
			r.headResult, r.failReason = StatusBadRequest, "bad host value"
			return false
		}
	}
	r.indexes.host = index
	return true
}
func (r *serverRequest_) checkIfModifiedSince(header *pair, index uint8) bool { // If-Modified-Since = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.ifModifiedSince, &r.unixTimes.ifModifiedSince)
}
func (r *serverRequest_) checkIfRange(header *pair, index uint8) bool { // If-Range = entity-tag / HTTP-date
	if r.indexes.ifRange != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated if-range"
		return false
	}
	if modTime, ok := clockParseHTTPDate(header.valueAt(r.input)); ok {
		r.unixTimes.ifRange = modTime
	}
	r.indexes.ifRange = index
	return true
}
func (r *serverRequest_) checkIfUnmodifiedSince(header *pair, index uint8) bool { // If-Unmodified-Since = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.ifUnmodifiedSince, &r.unixTimes.ifUnmodifiedSince)
}
func (r *serverRequest_) checkProxyAuthorization(header *pair, index uint8) bool { // Proxy-Authorization = auth-scheme [ 1*SP ( token68 / #auth-param ) ]
	// auth-scheme = token
	// token68     = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
	// auth-param  = token BWS "=" BWS ( token / quoted-string )
	// TODO
	if r.indexes.proxyAuthorization == 0 {
		r.indexes.proxyAuthorization = index
		return true
	} else {
		r.headResult, r.failReason = StatusBadRequest, "duplicated proxyAuthorization"
		return false
	}
}
func (r *serverRequest_) checkRange(header *pair, index uint8) bool { // Range = ranges-specifier
	if r.methodCode != MethodGET {
		r._delPrime(index)
		return true
	}
	if r.nRanges > 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated range"
		return false
	}
	// Range        = range-unit "=" range-set
	// range-set    = 1#range-spec
	// range-spec   = int-range / suffix-range
	// int-range    = first-pos "-" [ last-pos ]
	// suffix-range = "-" suffix-length
	rangeSet := header.valueAt(r.input)
	nPrefix := len(bytesBytesEqual) // bytes=
	if !bytes.Equal(rangeSet[0:nPrefix], bytesBytesEqual) {
		r.headResult, r.failReason = StatusBadRequest, "unsupported range unit"
		return false
	}
	rangeSet = rangeSet[nPrefix:]
	if len(rangeSet) == 0 {
		r.headResult, r.failReason = StatusBadRequest, "empty range-set"
		return false
	}
	var from, last int64 // inclusive
	state := 0           // select int-range or suffix-range
	for i, n := 0, len(rangeSet); i < n; i++ {
		b := rangeSet[i]
		switch state {
		case 0: // select int-range or suffix-range
			if b >= '0' && b <= '9' {
				from = int64(b - '0')
				state = 1 // int-range
			} else if b == '-' {
				from = -1
				last = 0
				state = 4 // suffix-range
			} else if b != ',' && b != ' ' {
				goto badRange
			}
		case 1: // in first-pos = 1*DIGIT
			for ; i < n; i++ {
				if b := rangeSet[i]; b >= '0' && b <= '9' {
					from = from*10 + int64(b-'0')
					if from < 0 {
						goto badRange
					}
				} else if b == '-' {
					state = 2 // select last-pos or not
					break
				} else {
					goto badRange
				}
			}
		case 2: // select last-pos or not
			if b >= '0' && b <= '9' { // last-pos
				last = int64(b - '0')
				state = 3 // first-pos "-" last-pos
			} else if b == ',' || b == ' ' { // not
				// got: first-pos "-"
				last = -1
				if !r._addRange(from, last) {
					return false
				}
				state = 0
			} else {
				goto badRange
			}
		case 3: // in last-pos = 1*DIGIT
			for ; i < n; i++ {
				if b := rangeSet[i]; b >= '0' && b <= '9' {
					last = last*10 + int64(b-'0')
					if last < 0 {
						goto badRange
					}
				} else if b == ',' || b == ' ' {
					if from > last {
						goto badRange
					}
					// got: first-pos "-" last-pos
					if !r._addRange(from, last) {
						return false
					}
					state = 0
					break
				} else {
					goto badRange
				}
			}
		case 4: // in suffix-length = 1*DIGIT
			for ; i < n; i++ {
				if b := rangeSet[i]; b >= '0' && b <= '9' {
					last = last*10 + int64(b-'0')
					if last < 0 {
						goto badRange
					}
				} else if b == ',' || b == ' ' {
					// got: "-" suffix-length
					if !r._addRange(from, last) {
						return false
					}
					state = 0
					break
				} else {
					goto badRange
				}
			}
		}
	}
	if state == 1 || state == 4 && rangeSet[len(rangeSet)-1] == '-' {
		goto badRange
	}
	if state == 2 {
		last = -1
	}
	if (state == 2 || state == 3 || state == 4) && !r._addRange(from, last) {
		return false
	}
	return true
badRange:
	r.headResult, r.failReason = StatusBadRequest, "invalid range"
	return false
}
func (r *serverRequest_) checkUserAgent(header *pair, index uint8) bool { // User-Agent = product *( RWS ( product / comment ) )
	if r.indexes.userAgent == 0 {
		r.indexes.userAgent = index
		return true
	} else {
		r.headResult, r.failReason = StatusBadRequest, "duplicated user-agent"
		return false
	}
}
func (r *serverRequest_) _addRange(from int64, last int64) bool {
	if r.nRanges == int8(cap(r.ranges)) {
		r.headResult, r.failReason = StatusBadRequest, "too many ranges"
		return false
	}
	r.ranges[r.nRanges] = rang{from, last}
	r.nRanges++
	return true
}

var ( // perfect hash table for important request headers
	serverRequestImportantHeaderTable = [16]struct {
		desc  // allowQuote, allowEmpty, allowParam, hasComment
		check func(*serverRequest_, []pair, uint8, uint8) bool
	}{ // accept-encoding accept-language cache-control connection content-encoding content-language expect forwarded if-match if-none-match te trailer transfer-encoding upgrade via x-forwarded-for
		0:  {desc{hashIfMatch, true, false, false, false, bytesIfMatch}, (*serverRequest_).checkIfMatch},
		1:  {desc{hashContentLanguage, false, false, false, false, bytesContentLanguage}, (*serverRequest_).checkContentLanguage},
		2:  {desc{hashVia, false, false, false, true, bytesVia}, (*serverRequest_).checkVia},
		3:  {desc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*serverRequest_).checkTransferEncoding}, // deliberately false
		4:  {desc{hashCacheControl, false, false, false, false, bytesCacheControl}, (*serverRequest_).checkCacheControl},
		5:  {desc{hashConnection, false, false, false, false, bytesConnection}, (*serverRequest_).checkConnection},
		6:  {desc{hashForwarded, false, false, false, false, bytesForwarded}, (*serverRequest_).checkForwarded}, // `for=192.0.2.60;proto=http;by=203.0.113.43` is not parameters
		7:  {desc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*serverRequest_).checkUpgrade},
		8:  {desc{hashXForwardedFor, false, false, false, false, bytesXForwardedFor}, (*serverRequest_).checkXForwardedFor},
		9:  {desc{hashExpect, false, false, true, false, bytesExpect}, (*serverRequest_).checkExpect},
		10: {desc{hashAcceptEncoding, false, true, true, false, bytesAcceptEncoding}, (*serverRequest_).checkAcceptEncoding},
		11: {desc{hashContentEncoding, false, false, false, false, bytesContentEncoding}, (*serverRequest_).checkContentEncoding},
		12: {desc{hashAcceptLanguage, false, false, true, false, bytesAcceptLanguage}, (*serverRequest_).checkAcceptLanguage},
		13: {desc{hashIfNoneMatch, true, false, false, false, bytesIfNoneMatch}, (*serverRequest_).checkIfNoneMatch},
		14: {desc{hashTE, false, false, true, false, bytesTE}, (*serverRequest_).checkTE},
		15: {desc{hashTrailer, false, false, false, false, bytesTrailer}, (*serverRequest_).checkTrailer},
	}
	serverRequestImportantHeaderFind = func(hash uint16) int { return (49454765 / int(hash)) % 16 }
)

func (r *serverRequest_) checkAcceptLanguage(pairs []pair, from uint8, edge uint8) bool { // Accept-Language = #( language-range [ weight ] )
	// language-range = <language-range, see [RFC4647], Section 2.1>
	// weight = OWS ";" OWS "q=" qvalue
	// qvalue = ( "0" [ "." *3DIGIT ] ) / ( "1" [ "." *3"0" ] )
	if r.zones.acceptLanguage.isEmpty() {
		r.zones.acceptLanguage.from = from
	}
	r.zones.acceptLanguage.edge = edge
	if IsDebug(2) {
		/*
			for i := from; i < edge; i++ {
				data := pairs[i].dataAt(r.input)
				Debugf("lang=%s\n", string(data))
			}
		*/
	}
	return true
}
func (r *serverRequest_) checkCacheControl(pairs []pair, from uint8, edge uint8) bool { // Cache-Control = #cache-directive
	// cache-directive = token [ "=" ( token / quoted-string ) ]
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *serverRequest_) checkExpect(pairs []pair, from uint8, edge uint8) bool { // Expect = #expectation
	// expectation = token [ "=" ( token / quoted-string ) parameters ]
	if r.versionCode >= Version1_1 {
		if r.zones.expect.isEmpty() {
			r.zones.expect.from = from
		}
		r.zones.expect.edge = edge
		for i := from; i < edge; i++ {
			data := pairs[i].dataAt(r.input)
			bytesToLower(data) // the Expect field-value is case-insensitive.
			if bytes.Equal(data, bytes100Continue) {
				r.expectContinue = true
			} else {
				// Unknown expectation, ignored.
			}
		}
	} else { // HTTP/1.0
		// RFC 7231 (section 5.1.1):
		// A server that receives a 100-continue expectation in an HTTP/1.0 request MUST ignore that expectation.
		for i := from; i < edge; i++ {
			pairs[i].zero() // since HTTP/1.0 doesn't support 1xx status codes, we delete the expect.
		}
	}
	return true
}
func (r *serverRequest_) checkForwarded(pairs []pair, from uint8, edge uint8) bool { // Forwarded = 1#forwarded-element
	if from == edge {
		r.headResult, r.failReason = StatusBadRequest, "forwarded = 1#forwarded-element"
		return false
	}
	// forwarded-element = [ forwarded-pair ] *( ";" [ forwarded-pair ] )
	// forwarded-pair    = token "=" value
	// value             = token / quoted-string
	if r.zones.forwarded.isEmpty() {
		r.zones.forwarded.from = from
	}
	r.zones.forwarded.edge = edge
	return true
}
func (r *serverRequest_) checkIfMatch(pairs []pair, from uint8, edge uint8) bool { // If-Match = "*" / #entity-tag
	return r._checkMatch(pairs, from, edge, &r.zones.ifMatch, &r.ifMatch)
}
func (r *serverRequest_) checkIfNoneMatch(pairs []pair, from uint8, edge uint8) bool { // If-None-Match = "*" / #entity-tag
	return r._checkMatch(pairs, from, edge, &r.zones.ifNoneMatch, &r.ifNoneMatch)
}
func (r *serverRequest_) checkTE(pairs []pair, from uint8, edge uint8) bool { // TE = #t-codings
	// t-codings = "trailers" / ( transfer-coding [ t-ranking ] )
	// t-ranking = OWS ";" OWS "q=" rank
	for i := from; i < edge; i++ {
		data := pairs[i].dataAt(r.input)
		bytesToLower(data)
		if bytes.Equal(data, bytesTrailers) {
			r.acceptTrailers = true
		} else if r.versionCode > Version1_1 {
			r.headResult, r.failReason = StatusBadRequest, "te codings other than trailers are not allowed in http/2 and http/3"
			return false
		}
	}
	return true
}
func (r *serverRequest_) checkUpgrade(pairs []pair, from uint8, edge uint8) bool { // Upgrade = #protocol
	if r.versionCode > Version1_1 {
		r.headResult, r.failReason = StatusBadRequest, "upgrade is only supported in http/1.1"
		return false
	}
	if r.methodCode == MethodCONNECT {
		// TODO: confirm this
		return true
	}
	if r.versionCode == Version1_1 {
		// protocol         = protocol-name ["/" protocol-version]
		// protocol-name    = token
		// protocol-version = token
		for i := from; i < edge; i++ {
			data := pairs[i].dataAt(r.input)
			bytesToLower(data)
			if bytes.Equal(data, bytesWebSocket) {
				r.upgradeSocket = true
			} else {
				// Unknown protocol. Ignored. We don't support "Upgrade: h2c" either.
			}
		}
	} else { // HTTP/1.0
		// RFC 7230 (section 6.7):
		// A server MUST ignore an Upgrade header field that is received in an HTTP/1.0 request.
		for i := from; i < edge; i++ {
			pairs[i].zero() // we delete it.
		}
	}
	return true
}
func (r *serverRequest_) checkXForwardedFor(pairs []pair, from uint8, edge uint8) bool { // X-Forwarded-For: <client>, <proxy1>, <proxy2>
	if from == edge {
		r.headResult, r.failReason = StatusBadRequest, "empty x-forwarded-for"
		return false
	}
	if r.zones.xForwardedFor.isEmpty() {
		r.zones.xForwardedFor.from = from
	}
	r.zones.xForwardedFor.edge = edge
	return true
}
func (r *serverRequest_) _checkMatch(pairs []pair, from uint8, edge uint8, zMatch *zone, match *int8) bool {
	if zMatch.isEmpty() {
		zMatch.from = from
	}
	zMatch.edge = edge
	for i := from; i < edge; i++ {
		data := pairs[i].dataAt(r.input)
		nMatch := *match // -1:*, 0:nonexist, >0:num
		if len(data) == 1 && data[0] == '*' {
			if nMatch != 0 {
				r.headResult, r.failReason = StatusBadRequest, "mix using of * and entity-tag"
				return false
			}
			*match = -1 // *
		} else { // entity-tag = [ weak ] DQUOTE *etagc DQUOTE
			if nMatch == -1 { // *
				r.headResult, r.failReason = StatusBadRequest, "mix using of entity-tag and *"
				return false
			}
			if nMatch > 16 {
				r.headResult, r.failReason = StatusBadRequest, "too many entity-tag"
				return false
			}
			*match++ // *match is 0 by default
		}
	}
	return true
}

func (r *serverRequest_) parseAuthority(from int32, edge int32, save bool) bool { // authority = host [ ":" port ]
	if save {
		r.authority.set(from, edge)
	}
	// host = IP-literal / IPv4address / reg-name
	// IP-literal = "[" ( IPv6address / IPvFuture  ) "]"
	// port = *DIGIT
	back, fore := from, from
	if r.input[back] == '[' { // IP-literal
		back++
		fore = back
		for fore < edge {
			if b := r.input[fore]; (b >= 'a' && b <= 'f') || (b >= '0' && b <= '9') || b == ':' {
				fore++
			} else if b == ']' {
				break
			} else {
				return false
			}
		}
		if fore == edge || fore-back == 1 { // "[]" is illegal
			return false
		}
		if save {
			r.hostname.set(back, fore)
		}
		fore++
		if fore == edge {
			return true
		}
		if r.input[fore] != ':' {
			return false
		}
	} else { // IPv4address or reg-name
		for fore < edge {
			if b := r.input[fore]; httpNchar[b] == 1 {
				fore++
			} else if b == ':' {
				break
			} else {
				return false
			}
		}
		if save {
			r.hostname.set(back, fore)
		}
		if fore == edge {
			return true
		}
	}
	// Now fore is at ':'. cases are: ":", ":88"
	back = fore
	fore++
	for fore < edge {
		if b := r.input[fore]; b >= '0' && b <= '9' {
			fore++
		} else {
			return false
		}
	}
	if n := fore - back; n > 6 { // max len(":65535") == 6
		return false
	} else if n > 1 && save { // ":" alone is ignored
		r.colonPort.set(back, fore)
	}
	return true
}
func (r *serverRequest_) parseCookie(cookieString span) bool { // cookie-string = cookie-pair *( ";" SP cookie-pair )
	// cookie-pair = token "=" cookie-value
	// cookie-value = *cookie-octet / ( DQUOTE *cookie-octet DQUOTE )
	// cookie-octet = %x21 / %x23-2B / %x2D-3A / %x3C-5B / %x5D-7E
	// exclude these: %x22=`"`  %2C=`,`  %3B=`;`  %5C=`\`
	cookie := &r.mainPair
	cookie.zero()
	cookie.kind = kindCookie
	cookie.place = placeInput // all received cookies are in r.input
	cookie.nameFrom = cookieString.from
	state := 0
	for p := cookieString.from; p < cookieString.edge; p++ {
		b := r.input[p]
		switch state {
		case 0: // expecting '=' to get cookie-name
			if b == '=' {
				if nameSize := p - cookie.nameFrom; nameSize > 0 && nameSize <= 255 {
					cookie.nameSize = uint8(nameSize)
					cookie.value.from = p + 1 // skip '='
				} else {
					r.headResult, r.failReason = StatusBadRequest, "cookie name out of range"
					return false
				}
				state = 1
			} else if httpTchar[b] != 0 {
				cookie.hash += uint16(b)
			} else {
				r.headResult, r.failReason = StatusBadRequest, "invalid cookie name"
				return false
			}
		case 1: // DQUOTE or not?
			if b == '"' {
				cookie.value.from++ // skip '"'
				state = 3
				continue
			}
			state = 2
			fallthrough
		case 2: // *cookie-octet, expecting ';'
			if b == ';' {
				cookie.value.edge = p
				if !r.addCookie(cookie) {
					return false
				}
				state = 5
			} else if b < 0x21 || b == '"' || b == ',' || b == '\\' || b > 0x7e {
				r.headResult, r.failReason = StatusBadRequest, "invalid cookie value"
				return false
			}
		case 3: // (DQUOTE *cookie-octet DQUOTE), expecting '"'
			if b == '"' {
				cookie.value.edge = p
				if !r.addCookie(cookie) {
					return false
				}
				state = 4
			} else if b < 0x20 || b == ';' || b == '\\' || b > 0x7e { // ` ` and `,` are allowed here!
				r.headResult, r.failReason = StatusBadRequest, "invalid cookie value"
				return false
			}
		case 4: // expecting ';'
			if b != ';' {
				r.headResult, r.failReason = StatusBadRequest, "invalid cookie separator"
				return false
			}
			state = 5
		case 5: // expecting SP
			if b != ' ' {
				r.headResult, r.failReason = StatusBadRequest, "invalid cookie SP"
				return false
			}
			cookie.hash = 0         // reset for next cookie
			cookie.nameFrom = p + 1 // skip ' '
			state = 0
		}
	}
	if state == 2 { // ';' not found
		cookie.value.edge = cookieString.edge
		if !r.addCookie(cookie) {
			return false
		}
	} else if state == 4 { // ';' not found
		if !r.addCookie(cookie) {
			return false
		}
	} else { // 0, 1, 3, 5
		r.headResult, r.failReason = StatusBadRequest, "invalid cookie string"
		return false
	}
	return true
}

func (r *serverRequest_) AcceptTrailers() bool { return r.acceptTrailers }
func (r *serverRequest_) UserAgent() string    { return string(r.UnsafeUserAgent()) }
func (r *serverRequest_) UnsafeUserAgent() []byte {
	if r.indexes.userAgent == 0 {
		return nil
	}
	return r.primes[r.indexes.userAgent].valueAt(r.input)
}
func (r *serverRequest_) getRanges() []rang {
	if r.nRanges == 0 {
		return nil
	}
	return r.ranges[:r.nRanges]
}

func (r *serverRequest_) addCookie(cookie *pair) bool { // as prime
	if edge, ok := r._addPrime(cookie); ok {
		r.cookies.edge = edge
		return true
	}
	r.headResult = StatusRequestHeaderFieldsTooLarge
	return false
}
func (r *serverRequest_) AddCookie(name string, value string) bool { // as extra
	return r.addExtra(name, value, 0, kindCookie)
}
func (r *serverRequest_) HasCookies() bool                  { return r.hasPairs(r.cookies, kindCookie) }
func (r *serverRequest_) AllCookies() (cookies [][2]string) { return r.allPairs(r.cookies, kindCookie) }
func (r *serverRequest_) C(name string) string {
	value, _ := r.Cookie(name)
	return value
}
func (r *serverRequest_) Cstr(name string, defaultValue string) string {
	if value, ok := r.Cookie(name); ok {
		return value
	}
	return defaultValue
}
func (r *serverRequest_) Cint(name string, defaultValue int) int {
	if value, ok := r.Cookie(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *serverRequest_) Cookie(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.cookies, kindCookie)
	return string(v), ok
}
func (r *serverRequest_) UnsafeCookie(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.cookies, kindCookie)
}
func (r *serverRequest_) Cookies(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.cookies, kindCookie)
}
func (r *serverRequest_) HasCookie(name string) bool {
	_, ok := r.getPair(name, 0, r.cookies, kindCookie)
	return ok
}
func (r *serverRequest_) DelCookie(name string) (deleted bool) {
	return r.delPair(name, 0, r.cookies, kindCookie)
}
func (r *serverRequest_) forCookies(callback func(cookie *pair, name []byte, value []byte) bool) bool {
	for i := r.cookies.from; i < r.cookies.edge; i++ {
		if cookie := &r.primes[i]; cookie.hash != 0 {
			if !callback(cookie, cookie.nameAt(r.input), cookie.valueAt(r.input)) {
				return false
			}
		}
	}
	if r.hasExtra[kindCookie] {
		for i := 0; i < len(r.extras); i++ {
			if extra := &r.extras[i]; extra.hash != 0 && extra.kind == kindCookie {
				if !callback(extra, extra.nameAt(r.array), extra.valueAt(r.array)) {
					return false
				}
			}
		}
	}
	return true
}

func (r *serverRequest_) TestConditions(modTime int64, etag []byte, asOrigin bool) (status int16, pass bool) { // to test preconditons intentionally
	// Get etag without ""
	if n := len(etag); n >= 2 && etag[0] == '"' && etag[n-1] == '"' {
		etag = etag[1 : n-1]
	}
	// See RFC 9110 (section 13.2.2).
	if asOrigin { // proxies ignore these tests.
		if r.ifMatch != 0 && !r._testIfMatch(etag) {
			return StatusPreconditionFailed, false
		}
		if r.ifMatch == 0 && r.indexes.ifUnmodifiedSince != 0 && !r._testIfUnmodifiedSince(modTime) {
			return StatusPreconditionFailed, false
		}
	}
	getOrHead := r.methodCode&(MethodGET|MethodHEAD) != 0
	if r.ifNoneMatch != 0 && !r._testIfNoneMatch(etag) {
		if getOrHead {
			return StatusNotModified, false
		} else {
			return StatusPreconditionFailed, false
		}
	}
	if getOrHead && r.ifNoneMatch == 0 && r.indexes.ifModifiedSince != 0 && !r._testIfModifiedSince(modTime) {
		return StatusNotModified, false
	}
	return StatusOK, true
}
func (r *serverRequest_) _testIfMatch(etag []byte) (pass bool) {
	if r.ifMatch == -1 { // *
		return true
	}
	for i := r.zones.ifMatch.from; i < r.zones.ifMatch.edge; i++ {
		header := &r.primes[i]
		if header.hash != hashIfMatch || !header.nameEqualBytes(r.input, bytesIfMatch) {
			continue
		}
		data := header.dataAt(r.input)
		if dataSize := len(data); !(dataSize >= 4 && data[0] == 'W' && data[1] == '/' && data[2] == '"' && data[dataSize-1] == '"') && bytes.Equal(data, etag) {
			return true
		}
	}
	// TODO: extra?
	return false
}
func (r *serverRequest_) _testIfNoneMatch(etag []byte) (pass bool) {
	if r.ifNoneMatch == -1 { // *
		return false
	}
	for i := r.zones.ifNoneMatch.from; i < r.zones.ifNoneMatch.edge; i++ {
		header := &r.primes[i]
		if header.hash != hashIfNoneMatch || !header.nameEqualBytes(r.input, bytesIfNoneMatch) {
			continue
		}
		if bytes.Equal(header.valueAt(r.input), etag) {
			return false
		}
	}
	// TODO: extra?
	return true
}
func (r *serverRequest_) _testIfModifiedSince(modTime int64) (pass bool) {
	return modTime > r.unixTimes.ifModifiedSince
}
func (r *serverRequest_) _testIfUnmodifiedSince(modTime int64) (pass bool) {
	return modTime <= r.unixTimes.ifUnmodifiedSince
}

func (r *serverRequest_) TestIfRanges(modTime int64, etag []byte, asOrigin bool) (pass bool) {
	if r.methodCode == MethodGET && r.nRanges > 0 && r.indexes.ifRange != 0 {
		if (r.unixTimes.ifRange == 0 && r._testIfRangeETag(etag)) || (r.unixTimes.ifRange != 0 && r._testIfRangeTime(modTime)) {
			return true // StatusPartialContent
		}
	}
	return false // StatusOK
}
func (r *serverRequest_) _testIfRangeETag(etag []byte) (pass bool) {
	ifRange := &r.primes[r.indexes.ifRange]
	data := ifRange.dataAt(r.input)
	if dataSize := len(data); !(dataSize >= 4 && data[0] == 'W' && data[1] == '/' && data[2] == '"' && data[dataSize-1] == '"') && bytes.Equal(data, etag) {
		return true
	}
	return false
}
func (r *serverRequest_) _testIfRangeTime(modTime int64) (pass bool) {
	return r.unixTimes.ifRange == modTime
}

func (r *serverRequest_) unsetHost() { // used by proxies
	r._delPrime(r.indexes.host) // zero safe
}

func (r *serverRequest_) HasContent() bool { return r.contentSize >= 0 || r.IsUnsized() }
func (r *serverRequest_) Content() string  { return string(r.UnsafeContent()) }
func (r *serverRequest_) UnsafeContent() []byte {
	if r.formKind == httpFormMultipart { // loading multipart form into memory is not allowed!
		return nil
	}
	return r.unsafeContent()
}

func (r *serverRequest_) parseHTMLForm() { // to populate r.forms and r.uploads
	if r.formKind == httpFormNotForm || r.formReceived {
		return
	}
	r.formReceived = true
	r.forms.from = uint8(len(r.primes))
	r.forms.edge = r.forms.from
	if r.formKind == httpFormURLEncoded { // application/x-www-form-urlencoded
		r._loadURLEncodedForm()
	} else { // multipart/form-data
		r._recvMultipartForm()
	}
}
func (r *serverRequest_) _loadURLEncodedForm() { // into memory entirely
	r.loadContent()
	if r.stream.isBroken() {
		return
	}
	var (
		state = 2 // to be consistent with r.recvControl() in HTTP/1
		octet byte
	)
	form := &r.mainPair
	form.zero()
	form.kind = kindForm
	form.place = placeArray // all received forms are placed in r.array
	form.nameFrom = r.arrayEdge
	for i := int64(0); i < r.receivedSize; i++ { // TODO: use a better algorithm to improve performance
		b := r.contentText[i]
		switch state {
		case 2: // expecting '=' to get a name
			if b == '=' {
				if nameSize := r.arrayEdge - form.nameFrom; nameSize <= 255 {
					form.nameSize = uint8(nameSize)
					form.value.from = r.arrayEdge
				} else {
					r.bodyResult, r.failReason = StatusBadRequest, "form name too long"
					return
				}
				state = 3
			} else if httpPchar[b] > 0 { // including '?'
				if b == '+' {
					b = ' ' // application/x-www-form-urlencoded encodes ' ' as '+'
				}
				form.hash += uint16(b)
				r.arrayPush(b)
			} else if b == '%' {
				state = 0x2f // '2' means from state 2
			} else {
				r.bodyResult, r.failReason = StatusBadRequest, "invalid form name"
				return
			}
		case 3: // expecting '&' to get a value
			if b == '&' {
				form.value.edge = r.arrayEdge
				if form.nameSize > 0 {
					r.addForm(form)
				}
				form.hash = 0 // reset for next form
				form.nameFrom = r.arrayEdge
				state = 2
			} else if httpPchar[b] > 0 { // including '?'
				if b == '+' {
					b = ' ' // application/x-www-form-urlencoded encodes ' ' as '+'
				}
				r.arrayPush(b)
			} else if b == '%' {
				state = 0x3f // '3' means from state 3
			} else {
				r.bodyResult, r.failReason = StatusBadRequest, "invalid form value"
				return
			}
		default: // expecting HEXDIG
			half, ok := byteFromHex(b)
			if !ok {
				r.bodyResult, r.failReason = StatusBadRequest, "invalid pct encoding"
				return
			}
			if state&0xf == 0xf { // expecting the first HEXDIG
				octet = half << 4
				state &= 0xf0 // this reserves last state and leads to the state of second HEXDIG
			} else { // expecting the second HEXDIG
				octet |= half
				if state == 0x20 { // in name
					form.hash += uint16(octet)
				}
				r.arrayPush(octet)
				state >>= 4 // restore last state
			}
		}
	}
	// Reaches end of content.
	if state == 3 { // '&' not found
		form.value.edge = r.arrayEdge
		if form.nameSize > 0 {
			r.addForm(form)
		}
	} else { // '=' not found, or incomplete pct-encoded
		r.bodyResult, r.failReason = StatusBadRequest, "incomplete pct-encoded"
	}
}
func (r *serverRequest_) _recvMultipartForm() { // into memory or tempFile. see RFC 7578: https://datatracker.ietf.org/doc/html/rfc7578
	r.pBack, r.pFore = 0, 0
	r.consumedSize = r.receivedSize
	if r.contentReceived { // (0, 64K1)
		// r.contentText is set, r.contentTextKind == webContentTextInput. r.formWindow refers to the exact r.contentText.
		r.formWindow = r.contentText
		r.formEdge = int32(len(r.formWindow))
	} else { // content is not received
		r.contentReceived = true
		switch content := r.recvContent(true).(type) { // retain
		case []byte: // (0, 64K1]. case happens when sized content <= 64K1
			r.contentText = content
			r.contentTextKind = webContentTextPool         // so r.contentText can be freed on end
			r.formWindow = r.contentText[0:r.receivedSize] // r.formWindow refers to the exact r.content.
			r.formEdge = int32(r.receivedSize)
		case tempFile: // [0, r.app.maxUploadContentSize]. case happens when sized content > 64K1, or content is unsized.
			r.contentFile = content.(*os.File)
			if r.receivedSize == 0 {
				return // unsized content can be empty
			}
			// We need a window to read and parse. An adaptive r.formWindow is used
			if r.receivedSize <= _4K {
				r.formWindow = Get4K()
			} else {
				r.formWindow = Get16K()
			}
			defer func() {
				PutNK(r.formWindow)
				r.formWindow = nil
			}()
			r.formEdge = 0     // no initial data, will fill below
			r.consumedSize = 0 // increases when we grow content
			if !r._growMultipartForm() {
				return
			}
		case error:
			// TODO: log err
			r.stream.markBroken()
			return
		}
	}
	template := r.UnsafeMake(3 + r.boundary.size() + 2) // \n--boundary--
	template[0], template[1], template[2] = '\n', '-', '-'
	n := 3 + copy(template[3:], r.input[r.boundary.from:r.boundary.edge])
	separator := template[0:n] // \n--boundary
	template[n], template[n+1] = '-', '-'
	for { // each part in multipart
		// Now r.formWindow is used for receiving --boundary-- EOL or --boundary EOL
		for r.formWindow[r.pFore] != '\n' {
			if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
				return
			}
		}
		if r.pBack == r.pFore {
			r.stream.markBroken()
			return
		}
		fore := r.pFore
		if fore >= 1 && r.formWindow[fore-1] == '\r' {
			fore--
		}
		if bytes.Equal(r.formWindow[r.pBack:fore], template[1:n+2]) { // end of multipart (--boundary--)
			// All parts are received.
			if IsDebug(2) {
				Debugln(r.arrayEdge, cap(r.array), string(r.array[0:r.arrayEdge]))
			}
			return
		} else if !bytes.Equal(r.formWindow[r.pBack:fore], template[1:n]) { // not start of multipart (--boundary)
			r.stream.markBroken()
			return
		}
		// Skip '\n'
		if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
			return
		}
		// r.pFore is at fields of current part.
		var part struct { // current part
			valid  bool     // true if "name" parameter in "content-disposition" field is found
			isFile bool     // true if "filename" parameter in "content-disposition" field is found
			hash   uint16   // name hash
			name   span     // to r.array. like: "avatar"
			base   span     // to r.array. like: "michael.jpg", or empty if part is not a file
			type_  span     // to r.array. like: "image/jpeg", or empty if part is not a file
			path   span     // to r.array. like: "/path/to/391384576", or empty if part is not a file
			osFile *os.File // if part is a file, this is used
			form   pair     // if part is a form, this is used
			upload Upload   // if part is a file, this is used. zeroed
		}
		part.form.kind = kindForm
		part.form.place = placeArray // all received forms are placed in r.array
		for {                        // each field in current part
			// End of part fields?
			if b := r.formWindow[r.pFore]; b == '\r' {
				if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
					return
				}
				if r.formWindow[r.pFore] != '\n' {
					r.stream.markBroken()
					return
				}
				break
			} else if b == '\n' {
				break
			}
			r.pBack = r.pFore // now r.formWindow is used for receiving field-name and onward
			for {             // field name
				b := r.formWindow[r.pFore]
				if t := httpTchar[b]; t == 1 {
					// Fast path, do nothing
				} else if t == 2 { // A-Z
					r.formWindow[r.pFore] = b + 0x20 // to lower
				} else if t == 3 { // '_'
					// For forms, do nothing
				} else if b == ':' {
					break
				} else {
					r.stream.markBroken()
					return
				}
				if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
					return
				}
			}
			if r.pBack == r.pFore { // field-name cannot be empty
				r.stream.markBroken()
				return
			}
			r.pFieldName.set(r.pBack, r.pFore) // in case of sliding r.formWindow when r._growMultipartForm()
			// Skip ':'
			if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
				return
			}
			// Skip OWS before field value
			for r.formWindow[r.pFore] == ' ' || r.formWindow[r.pFore] == '\t' {
				if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
					return
				}
			}
			r.pBack = r.pFore
			// Now r.formWindow is used for receiving field-value and onward. at this time we can still use r.pFieldName, no risk of sliding
			if fieldName := r.formWindow[r.pFieldName.from:r.pFieldName.edge]; bytes.Equal(fieldName, bytesContentDisposition) { // content-disposition
				// form-data; name="avatar"; filename="michael.jpg"
				for r.formWindow[r.pFore] != ';' {
					if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
				if r.pBack == r.pFore || !bytes.Equal(r.formWindow[r.pBack:r.pFore], bytesFormData) {
					r.stream.markBroken()
					return
				}
				r.pBack = r.pFore // now r.formWindow is used for receiving parameters and onward
				for r.formWindow[r.pFore] != '\n' {
					if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
				fore := r.pFore
				if r.formWindow[fore-1] == '\r' {
					fore--
				}
				// Skip OWS after field value
				for r.formWindow[fore-1] == ' ' || r.formWindow[fore-1] == '\t' {
					fore--
				}
				paras := make([]para, 2) // for name & filename. won't escape to heap
				n, ok := r._parseParas(r.formWindow, r.pBack, fore, paras)
				if !ok {
					r.stream.markBroken()
					return
				}
				for i := 0; i < n; i++ { // each para in field (; name="avatar"; filename="michael.jpg")
					para := &paras[i]
					if paraName := r.formWindow[para.name.from:para.name.edge]; bytes.Equal(paraName, bytesName) { // name="avatar"
						if n := para.value.size(); n == 0 || n > 255 {
							r.stream.markBroken()
							return
						}
						part.valid = true // as long as we got a name, this part is valid
						part.name.from = r.arrayEdge
						if !r.arrayCopy(r.formWindow[para.value.from:para.value.edge]) { // add "avatar"
							r.stream.markBroken()
							return
						}
						part.name.edge = r.arrayEdge
						// TODO: Is this a good implementation? If size is too large, just use bytes.Equal? Use a special hash value to hint this?
						for p := para.value.from; p < para.value.edge; p++ {
							part.hash += uint16(r.formWindow[p])
						}
					} else if bytes.Equal(paraName, bytesFilename) { // filename="michael.jpg"
						if n := para.value.size(); n == 0 || n > 255 {
							r.stream.markBroken()
							return
						}
						part.isFile = true

						part.base.from = r.arrayEdge
						if !r.arrayCopy(r.formWindow[para.value.from:para.value.edge]) { // add "michael.jpg"
							r.stream.markBroken()
							return
						}
						part.base.edge = r.arrayEdge

						part.path.from = r.arrayEdge
						if !r.arrayCopy(risky.ConstBytes(r.app.SaveContentFilesDir())) { // add "/path/to/"
							r.stream.markBroken()
							return
						}
						tempName := r.stream.buffer256() // buffer is enough for tempName
						from, edge := r.stream.makeTempName(tempName, r.recvTime.Unix())
						if !r.arrayCopy(tempName[from:edge]) { // add "391384576"
							r.stream.markBroken()
							return
						}
						part.path.edge = r.arrayEdge // pathSize is ensured to be <= 255.
					} else {
						// Other parameters are invalid.
						r.stream.markBroken()
						return
					}
				}
			} else if bytes.Equal(fieldName, bytesContentType) { // content-type
				// image/jpeg
				for r.formWindow[r.pFore] != '\n' {
					if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
				fore := r.pFore
				if r.formWindow[fore-1] == '\r' {
					fore--
				}
				// Skip OWS after field value
				for r.formWindow[fore-1] == ' ' || r.formWindow[fore-1] == '\t' {
					fore--
				}
				if n := fore - r.pBack; n == 0 || n > 255 {
					r.stream.markBroken()
					return
				}
				part.type_.from = r.arrayEdge
				if !r.arrayCopy(r.formWindow[r.pBack:fore]) { // add "image/jpeg"
					r.stream.markBroken()
					return
				}
				part.type_.edge = r.arrayEdge
			} else { // other fields are ignored
				for r.formWindow[r.pFore] != '\n' {
					if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
			}
			// Skip '\n' and goto next field or end of fields
			if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
				return
			}
		}
		if !part.valid { // no valid fields
			r.stream.markBroken()
			return
		}
		// Now all fields of the part are received. Skip end of fields and goto part data
		if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm() {
			return
		}
		if part.isFile {
			// TODO: upload code
			part.upload.hash = part.hash
			part.upload.nameSize, part.upload.nameFrom = uint8(part.name.size()), part.name.from
			part.upload.baseSize, part.upload.baseFrom = uint8(part.base.size()), part.base.from
			part.upload.typeSize, part.upload.typeFrom = uint8(part.type_.size()), part.type_.from
			part.upload.pathSize, part.upload.pathFrom = uint8(part.path.size()), part.path.from
			if osFile, err := os.OpenFile(risky.WeakString(r.array[part.path.from:part.path.edge]), os.O_RDWR|os.O_CREATE, 0644); err == nil {
				if IsDebug(2) {
					Debugln("OPENED")
				}
				part.osFile = osFile
			} else {
				if IsDebug(2) {
					Debugln(err.Error())
				}
				part.osFile = nil
			}
		} else { // part must be a form
			part.form.hash = part.hash
			part.form.nameFrom = part.name.from
			part.form.nameSize = uint8(part.name.size())
			part.form.value.from = r.arrayEdge
		}
		r.pBack = r.pFore // now r.formWindow is used for receiving part data and onward
		for {             // each partial in current part
			partial := r.formWindow[r.pBack:r.formEdge]
			r.pFore = r.formEdge
			mode := 0 // by default, we assume end of part ("\n--boundary") is not in partial
			var i int
			if i = bytes.Index(partial, separator); i >= 0 {
				mode = 1 // end of part ("\n--boundary") is found in partial
			} else if i = bytes.LastIndexByte(partial, '\n'); i >= 0 && bytes.HasPrefix(separator, partial[i:]) {
				mode = 2 // partial ends with prefix of end of part ("\n--boundary")
			}
			if mode > 0 { // found "\n" at i
				r.pFore = r.pBack + int32(i)
				if r.pFore > r.pBack && r.formWindow[r.pFore-1] == '\r' {
					r.pFore--
				}
				partial = r.formWindow[r.pBack:r.pFore] // pure data
			}
			if !part.isFile {
				if !r.arrayCopy(partial) { // join form value
					r.stream.markBroken()
					return
				}
				if mode == 1 { // form part ends
					part.form.value.edge = r.arrayEdge
					r.addForm(&part.form)
				}
			} else if part.osFile != nil {
				part.osFile.Write(partial)
				if mode == 1 { // file part ends
					r.addUpload(&part.upload)
					part.osFile.Close()
					if IsDebug(2) {
						Debugln("CLOSED")
					}
				}
			}
			if mode == 1 {
				r.pBack += int32(i + 1) // at the first '-' of "--boundary"
				r.pFore = r.pBack       // next part starts here
				break                   // part is received.
			}
			if mode == 2 {
				r.pBack = r.pFore // from EOL (\r or \n). need more and continue
			} else { // mode == 0
				r.pBack, r.formEdge = 0, 0 // pure data, clean r.formWindow. need more and continue
			}
			// Grow more
			if !r._growMultipartForm() {
				return
			}
		}
	}
}
func (r *serverRequest_) _growMultipartForm() bool { // caller needs more data from content file
	if r.consumedSize == r.receivedSize || (r.formEdge == int32(len(r.formWindow)) && r.pBack == 0) {
		r.stream.markBroken()
		return false
	}
	if r.pBack > 0 { // have useless data. slide to start
		copy(r.formWindow, r.formWindow[r.pBack:r.formEdge])
		r.formEdge -= r.pBack
		r.pFore -= r.pBack
		if r.pFieldName.notEmpty() {
			r.pFieldName.sub(r.pBack) // for fields in multipart/form-data, not for trailers
		}
		r.pBack = 0
	}
	n, err := r.contentFile.Read(r.formWindow[r.formEdge:])
	r.formEdge += int32(n)
	r.consumedSize += int64(n)
	if err == io.EOF {
		if r.consumedSize == r.receivedSize {
			err = nil
		} else {
			err = io.ErrUnexpectedEOF
		}
	}
	if err != nil {
		r.stream.markBroken()
		return false
	}
	return true
}
func (r *serverRequest_) _parseParas(p []byte, from int32, edge int32, paras []para) (int, bool) {
	// param-string = *( OWS ";" OWS param-pair )
	// param-pair   = token "=" param-value
	// param-value  = *param-octet / ( DQUOTE *param-octet DQUOTE )
	// param-octet  = ?
	back, fore := from, from
	nAdd := 0
	for {
		nSemic := 0
		for fore < edge {
			if b := p[fore]; b == ' ' || b == '\t' {
				fore++
			} else if b == ';' {
				nSemic++
				fore++
			} else {
				break
			}
		}
		if fore == edge || nSemic != 1 {
			// `; ` and ` ` and `;;` are invalid
			return nAdd, false
		}
		back = fore // for name
		for fore < edge {
			if b := p[fore]; b == '=' {
				break
			} else if b == ';' || b == ' ' || b == '\t' {
				// `; a; ` is invalid
				return nAdd, false
			} else {
				fore++
			}
		}
		if fore == edge || back == fore {
			// `; a` and `; ="b"` are invalid
			return nAdd, false
		}
		para := &paras[nAdd]
		para.name.set(back, fore)
		fore++ // skip '='
		if fore == edge {
			para.value.zero()
			nAdd++
			return nAdd, true
		}
		back = fore
		if p[fore] == '"' {
			fore++
			for fore < edge && p[fore] != '"' {
				fore++
			}
			if fore == edge {
				para.value.set(back, fore) // value is "...
			} else {
				para.value.set(back+1, fore) // strip ""
				fore++
			}
		} else {
			for fore < edge && p[fore] != ';' && p[fore] != ' ' && p[fore] != '\t' {
				fore++
			}
			para.value.set(back, fore)
		}
		nAdd++
		if nAdd == len(paras) || fore == edge {
			return nAdd, true
		}
	}
}

func (r *serverRequest_) addForm(form *pair) bool { // as prime
	if edge, ok := r._addPrime(form); ok {
		r.forms.edge = edge
		return true
	}
	r.bodyResult, r.failReason = StatusURITooLong, "too many forms"
	return false
}
func (r *serverRequest_) AddForm(name string, value string) bool { // as extra
	return r.addExtra(name, value, 0, kindForm)
}
func (r *serverRequest_) HasForms() bool {
	r.parseHTMLForm()
	return r.hasPairs(r.forms, kindForm)
}
func (r *serverRequest_) AllForms() (forms [][2]string) {
	r.parseHTMLForm()
	return r.allPairs(r.forms, kindForm)
}
func (r *serverRequest_) F(name string) string {
	value, _ := r.Form(name)
	return value
}
func (r *serverRequest_) Fstr(name string, defaultValue string) string {
	if value, ok := r.Form(name); ok {
		return value
	}
	return defaultValue
}
func (r *serverRequest_) Fint(name string, defaultValue int) int {
	if value, ok := r.Form(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *serverRequest_) Form(name string) (value string, ok bool) {
	r.parseHTMLForm()
	v, ok := r.getPair(name, 0, r.forms, kindForm)
	return string(v), ok
}
func (r *serverRequest_) UnsafeForm(name string) (value []byte, ok bool) {
	r.parseHTMLForm()
	return r.getPair(name, 0, r.forms, kindForm)
}
func (r *serverRequest_) Forms(name string) (values []string, ok bool) {
	r.parseHTMLForm()
	return r.getPairs(name, 0, r.forms, kindForm)
}
func (r *serverRequest_) HasForm(name string) bool {
	r.parseHTMLForm()
	_, ok := r.getPair(name, 0, r.forms, kindForm)
	return ok
}
func (r *serverRequest_) DelForm(name string) (deleted bool) {
	r.parseHTMLForm()
	return r.delPair(name, 0, r.forms, kindForm)
}

func (r *serverRequest_) addUpload(upload *Upload) {
	if len(r.uploads) == cap(r.uploads) {
		if cap(r.uploads) == cap(r.stockUploads) {
			uploads := make([]Upload, 0, 16)
			r.uploads = append(uploads, r.uploads...)
		} else if cap(r.uploads) == 16 {
			uploads := make([]Upload, 0, 128)
			r.uploads = append(uploads, r.uploads...)
		} else {
			// Ignore too many uploads
			return
		}
	}
	r.uploads = append(r.uploads, *upload)
}
func (r *serverRequest_) HasUploads() bool {
	r.parseHTMLForm()
	return len(r.uploads) != 0
}
func (r *serverRequest_) AllUploads() (uploads []*Upload) {
	r.parseHTMLForm()
	for i := 0; i < len(r.uploads); i++ {
		upload := &r.uploads[i]
		upload.setMeta(r.array)
		uploads = append(uploads, upload)
	}
	return uploads
}
func (r *serverRequest_) U(name string) *Upload {
	upload, _ := r.Upload(name)
	return upload
}
func (r *serverRequest_) Upload(name string) (upload *Upload, ok bool) {
	r.parseHTMLForm()
	if n := len(r.uploads); n > 0 && name != "" {
		hash := stringHash(name)
		for i := 0; i < n; i++ {
			if upload := &r.uploads[i]; upload.hash == hash && upload.nameEqualString(r.array, name) {
				upload.setMeta(r.array)
				return upload, true
			}
		}
	}
	return
}
func (r *serverRequest_) Uploads(name string) (uploads []*Upload, ok bool) {
	r.parseHTMLForm()
	if n := len(r.uploads); n > 0 && name != "" {
		hash := stringHash(name)
		for i := 0; i < n; i++ {
			if upload := &r.uploads[i]; upload.hash == hash && upload.nameEqualString(r.array, name) {
				upload.setMeta(r.array)
				uploads = append(uploads, upload)
			}
		}
		if len(uploads) > 0 {
			ok = true
		}
	}
	return
}
func (r *serverRequest_) HasUpload(name string) bool {
	r.parseHTMLForm()
	_, ok := r.Upload(name)
	return ok
}

func (r *serverRequest_) applyTrailer(index uint8) bool {
	//trailer := &r.primes[index]
	// TODO: Pseudo-header fields MUST NOT appear in a trailer section.
	return true
}

func (r *serverRequest_) arrayCopy(p []byte) bool {
	if len(p) > 0 {
		edge := r.arrayEdge + int32(len(p))
		if edge < r.arrayEdge { // overflow
			return false
		}
		if r.app != nil && edge > r.app.maxMemoryContentSize {
			return false
		}
		if !r._growArray(int32(len(p))) {
			return false
		}
		r.arrayEdge += int32(copy(r.array[r.arrayEdge:], p))
	}
	return true
}

func (r *serverRequest_) saveContentFilesDir() string {
	if r.app != nil {
		return r.app.SaveContentFilesDir()
	} else {
		return r.stream.webAgent().SaveContentFilesDir()
	}
}

func (r *serverRequest_) hookReviser(reviser Reviser) {
	r.hasRevisers = true
	r.revisers[reviser.Rank()] = reviser.ID() // revisers are placed to fixed position, by their ranks.
}

func (r *serverRequest_) unsafeVariable(index int16) []byte {
	return serverRequestVariables[index](r)
}

var serverRequestVariables = [...]func(*serverRequest_) []byte{ // keep sync with varCodes in engine.go
	(*serverRequest_).UnsafeMethod,      // method
	(*serverRequest_).UnsafeScheme,      // scheme
	(*serverRequest_).UnsafeAuthority,   // authority
	(*serverRequest_).UnsafeHostname,    // hostname
	(*serverRequest_).UnsafeColonPort,   // colonPort
	(*serverRequest_).UnsafePath,        // path
	(*serverRequest_).UnsafeURI,         // uri
	(*serverRequest_).UnsafeEncodedPath, // encodedPath
	(*serverRequest_).UnsafeQueryString, // queryString
	(*serverRequest_).UnsafeContentType, // contentType
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

// Response is the interface for *http[1-3]Response and *hweb[1-2]Response.
type Response interface {
	Request() Request

	SetStatus(status int16) error
	Status() int16

	MakeETagFrom(modTime int64, fileSize int64) ([]byte, bool) // with `""`
	SetExpires(expires int64) bool
	SetLastModified(lastModified int64) bool
	AddContentType(contentType string) bool
	AddHTTPSRedirection(authority string) bool
	AddHostnameRedirection(hostname string) bool
	AddDirectoryRedirection() bool

	SetCookie(cookie *Cookie) bool

	AddHeader(name string, value string) bool
	AddHeaderBytes(name []byte, value []byte) bool
	Header(name string) (value string, ok bool)
	HasHeader(name string) bool
	DelHeader(name string) bool
	DelHeaderBytes(name []byte) bool

	IsSent() bool
	SetSendTimeout(timeout time.Duration) // to defend against slowloris attack

	Send(content string) error
	SendBytes(content []byte) error
	SendJSON(content any) error
	SendFile(contentPath string) error
	SendBadRequest(content []byte) error                     // 400
	SendForbidden(content []byte) error                      // 403
	SendNotFound(content []byte) error                       // 404
	SendMethodNotAllowed(allow string, content []byte) error // 405
	SendInternalServerError(content []byte) error            // 500
	SendNotImplemented(content []byte) error                 // 501
	SendBadGateway(content []byte) error                     // 502
	SendGatewayTimeout(content []byte) error                 // 504

	Echo(chunk string) error
	EchoBytes(chunk []byte) error
	EchoFile(chunkPath string) error

	AddTrailer(name string, value string) bool
	AddTrailerBytes(name []byte, value []byte) bool

	// Internal only
	addHeader(name []byte, value []byte) bool
	header(name []byte) (value []byte, ok bool)
	hasHeader(name []byte) bool
	delHeader(name []byte) bool
	setConnectionClose()
	copyHeadFrom(resp clientResponse, viaName []byte) bool // used by proxies
	sendText(content []byte) error
	sendFile(content *os.File, info os.FileInfo, shut bool) error // will close content after sent
	sendChain() error                                             // content
	echoHeaders() error
	echoChain() error // chunks
	addTrailer(name []byte, value []byte) bool
	endUnsized() error
	finalizeUnsized() error
	pass1xx(resp clientResponse) bool         // used by proxies
	pass(resp webIn) error                    // used by proxies
	post(content any, hasTrailers bool) error // used by proxies
	copyTailFrom(resp clientResponse) bool    // used by proxies
	hookReviser(reviser Reviser)
	unsafeMake(size int) []byte
}

// serverResponse_ is the mixin for http[1-3]Response and hweb[1-2]Response.
type serverResponse_ struct { // outgoing. needs building
	// Mixins
	webOut_ // outgoing web message
	// Assocs
	request Request // *http[1-3]Request or *hweb[1-2]Request
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	status    int16    // 200, 302, 404, 500, ...
	start     [16]byte // exactly 16 bytes for "HTTP/1.1 xxx ?\r\n". also used by HTTP/2 and HTTP/3, but shorter
	unixTimes struct {
		expires      int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
		lastModified int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
	}
	// Stream states (zeros)
	app             *App // associated app
	serverResponse0      // all values must be zero by default in this struct!
}
type serverResponse0 struct { // for fast reset, entirely
	indexes struct {
		expires      uint8
		lastModified uint8
	}
	revisers [32]uint8 // reviser ids which will apply on this response. indexed by reviser order
}

func (r *serverResponse_) onUse(versionCode uint8) { // for non-zeros
	r.webOut_.onUse(versionCode, false) // asRequest = false
	r.status = StatusOK
	r.unixTimes.expires = -1      // not set
	r.unixTimes.lastModified = -1 // not set
}
func (r *serverResponse_) onEnd() { // for zeros
	r.app = nil
	r.serverResponse0 = serverResponse0{}
	r.webOut_.onEnd()
}

func (r *serverResponse_) Request() Request { return r.request }

func (r *serverResponse_) control() []byte { // only for HTTP/2 and HTTP/3. HTTP/1 has its own control()
	var start []byte
	if r.status >= int16(len(webControls)) || webControls[r.status] == nil {
		copy(r.start[:], webTemplate[:])
		r.start[8] = byte(r.status/100 + '0')
		r.start[9] = byte(r.status/10%10 + '0')
		r.start[10] = byte(r.status%10 + '0')
		start = r.start[:len(webTemplate)]
	} else {
		start = webControls[r.status]
	}
	return start
}

func (r *serverResponse_) SetStatus(status int16) error {
	if status >= 200 && status < 1000 {
		r.status = status
		if status == StatusNoContent {
			r.forbidFraming = true
			r.forbidContent = true
		} else if status == StatusNotModified {
			// A server MAY send a Content-Length header field in a 304 (Not Modified) response to a conditional GET request.
			r.forbidFraming = true // we forbid it.
			r.forbidContent = true
		}
		return nil
	} else { // 1xx are not allowed to set through SetStatus()
		return webOutUnknownStatus
	}
}
func (r *serverResponse_) Status() int16 { return r.status }

func (r *serverResponse_) MakeETagFrom(modTime int64, fileSize int64) ([]byte, bool) { // with ""
	if modTime < 0 || fileSize < 0 {
		return nil, false
	}
	p := r.unsafeMake(32)
	p[0] = '"'
	etag := p[1:]
	n := i64ToHex(modTime, etag)
	etag[n] = '-'
	n++
	if n > 13 {
		return nil, false
	}
	n = 1 + n + i64ToHex(fileSize, etag[n:])
	p[n] = '"'
	return p[0 : n+1], true
}
func (r *serverResponse_) SetExpires(expires int64) bool {
	return r._setUnixTime(&r.unixTimes.expires, &r.indexes.expires, expires)
}
func (r *serverResponse_) SetLastModified(lastModified int64) bool {
	return r._setUnixTime(&r.unixTimes.lastModified, &r.indexes.lastModified, lastModified)
}

func (r *serverResponse_) SendBadRequest(content []byte) error { // 400
	return r.sendError(StatusBadRequest, content)
}
func (r *serverResponse_) SendForbidden(content []byte) error { // 403
	return r.sendError(StatusForbidden, content)
}
func (r *serverResponse_) SendNotFound(content []byte) error { // 404
	return r.sendError(StatusNotFound, content)
}
func (r *serverResponse_) SendMethodNotAllowed(allow string, content []byte) error { // 405
	r.AddHeaderBytes(bytesAllow, risky.ConstBytes(allow))
	return r.sendError(StatusMethodNotAllowed, content)
}
func (r *serverResponse_) SendInternalServerError(content []byte) error { // 500
	return r.sendError(StatusInternalServerError, content)
}
func (r *serverResponse_) SendNotImplemented(content []byte) error { // 501
	return r.sendError(StatusNotImplemented, content)
}
func (r *serverResponse_) SendBadGateway(content []byte) error { // 502
	return r.sendError(StatusBadGateway, content)
}
func (r *serverResponse_) SendGatewayTimeout(content []byte) error { // 504
	return r.sendError(StatusGatewayTimeout, content)
}
func (r *serverResponse_) sendError(status int16, content []byte) error {
	if err := r._beforeSend(); err != nil {
		return err
	}
	if err := r.SetStatus(status); err != nil {
		return err
	}
	if content == nil {
		content = webErrorPages[status]
	}
	r.piece.SetText(content)
	r.chain.PushTail(&r.piece)
	r.contentSize = int64(len(content))
	return r.shell.sendChain()
}
func (r *serverResponse_) doSend() error {
	/*
		if r.hasRevisers {
			resp := r.shell.(Response)
			// Travel through revisers
			for _, id := range r.revisers { // revise headers
				if id == 0 { // id of effective reviser is ensured to be > 0
					continue
				}
				reviser := r.app.reviserByID(id)
				reviser.BeforeSend(resp.Request(), resp)
			}
			for _, id := range r.revisers { // revise sized content
				if id == 0 {
					continue
				}
				reviser := r.app.reviserByID(id)
				reviser.OnSend(resp.Request(), resp, &r.chain)
			}
			// Because r.chain may be altered/replaced by revisers, content size must be recalculated
			if contentSize, ok := r.chain.Size(); ok {
				r.contentSize = contentSize
			} else {
				return webOutTooLarge
			}
		}
	*/
	return r.shell.sendChain()
}

func (r *serverResponse_) beforeRevise() {
	resp := r.shell.(Response)
	for _, id := range r.revisers { // revise headers
		if id == 0 { // id of effective reviser is ensured to be > 0
			continue
		}
		reviser := r.app.reviserByID(id)
		reviser.BeforeEcho(resp.Request(), resp)
	}
}
func (r *serverResponse_) doEcho() error {
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	r.chain.PushTail(&r.piece)
	defer r.chain.free()
	/*
		if r.hasRevisers {
			resp := r.shell.(Response)
			for _, id := range r.revisers { // revise unsized content
				if id == 0 { // id of effective reviser is ensured to be > 0
					continue
				}
				reviser := r.app.reviserByID(id)
				reviser.OnEcho(resp.Request(), resp, &r.chain)
			}
		}
	*/
	return r.shell.echoChain()
}
func (r *serverResponse_) endUnsized() error {
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	/*
		if r.hasRevisers {
			resp := r.shell.(Response)
			for _, id := range r.revisers { // finish content
				if id == 0 { // id of effective reviser is ensured to be > 0
					continue
				}
				reviser := r.app.reviserByID(id)
				reviser.FinishEcho(resp.Request(), resp)
			}
		}
	*/
	return r.shell.finalizeUnsized()
}

func (r *serverResponse_) copyHeadFrom(resp clientResponse, viaName []byte) bool { // used by proxies
	resp.delHopHeaders()

	// copy control (:status)
	r.SetStatus(resp.Status())

	// copy selective forbidden headers (excluding set-cookie, which is copied directly) from resp

	// copy remaining headers from resp
	if !resp.forHeaders(func(header *pair, name []byte, value []byte) bool {
		if header.hash == hashSetCookie && bytes.Equal(name, bytesSetCookie) { // set-cookie is copied directly
			return r.shell.addHeader(name, value)
		}
		return r.shell.insertHeader(header.hash, name, value)
	}) {
		return false
	}

	return true
}

var ( // perfect hash table for response critical headers
	serverResponseCriticalHeaderTable = [10]struct {
		hash uint16
		name []byte
		fAdd func(*serverResponse_, []byte) (ok bool)
		fDel func(*serverResponse_) (deleted bool)
	}{ // connection content-length content-type date expires last-modified server set-cookie transfer-encoding upgrade
		0: {hashServer, bytesServer, nil, nil},       // forbidden
		1: {hashSetCookie, bytesSetCookie, nil, nil}, // forbidden
		2: {hashUpgrade, bytesUpgrade, nil, nil},     // forbidden
		3: {hashDate, bytesDate, (*serverResponse_).appendDate, (*serverResponse_).deleteDate},
		4: {hashTransferEncoding, bytesTransferEncoding, nil, nil}, // forbidden
		5: {hashConnection, bytesConnection, nil, nil},             // forbidden
		6: {hashLastModified, bytesLastModified, (*serverResponse_).appendLastModified, (*serverResponse_).deleteLastModified},
		7: {hashExpires, bytesExpires, (*serverResponse_).appendExpires, (*serverResponse_).deleteExpires},
		8: {hashContentLength, bytesContentLength, nil, nil}, // forbidden
		9: {hashContentType, bytesContentType, (*serverResponse_).appendContentType, (*serverResponse_).deleteContentType},
	}
	serverResponseCriticalHeaderFind = func(hash uint16) int { return (113100 / int(hash)) % 10 }
)

func (r *serverResponse_) insertHeader(hash uint16, name []byte, value []byte) bool {
	h := &serverResponseCriticalHeaderTable[serverResponseCriticalHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(h.name, name) {
		if h.fAdd == nil { // mainly because this header is forbidden to insert
			return true // pretend to be successful
		}
		return h.fAdd(r, value)
	}
	return r.shell.addHeader(name, value)
}
func (r *serverResponse_) appendExpires(expires []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.expires, &r.indexes.expires, bytesExpires, expires)
}
func (r *serverResponse_) appendLastModified(lastModified []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.lastModified, &r.indexes.lastModified, bytesLastModified, lastModified)
}

func (r *serverResponse_) removeHeader(hash uint16, name []byte) bool {
	h := &serverResponseCriticalHeaderTable[serverResponseCriticalHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(h.name, name) {
		if h.fDel == nil { // mainly because this header is forbidden to remove
			return true // pretend to be successful
		}
		return h.fDel(r)
	}
	return r.shell.delHeader(name)
}
func (r *serverResponse_) deleteExpires() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.expires, &r.indexes.expires)
}
func (r *serverResponse_) deleteLastModified() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.lastModified, &r.indexes.lastModified)
}

func (r *serverResponse_) copyTailFrom(resp clientResponse) bool { // used by proxies
	return resp.forTrailers(func(trailer *pair, name []byte, value []byte) bool {
		return r.shell.addTrailer(name, value)
	})
}

func (r *serverResponse_) hookReviser(reviser Reviser) {
	r.hasRevisers = true
	r.revisers[reviser.Rank()] = reviser.ID() // revisers are placed to fixed position, by their ranks.
}

// Cookie is a "set-cookie" sent to client.
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
	expires = expires.UTC()
	if expires.Year() < 1601 {
		c.invalid = true
		return false
	}
	c.expires = expires
	return true
}
func (c *Cookie) SetMaxAge(maxAge int64)  { c.maxAge = maxAge }
func (c *Cookie) SetSecure()              { c.secure = true }
func (c *Cookie) SetHttpOnly()            { c.httpOnly = true }
func (c *Cookie) SetSameSiteStrict()      { c.sameSite = "Strict" }
func (c *Cookie) SetSameSiteLax()         { c.sameSite = "Lax" }
func (c *Cookie) SetSameSiteNone()        { c.sameSite = "None" }
func (c *Cookie) SetSameSite(mode string) { c.sameSite = mode }

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
		i += clockWriteHTTPDate(p[i:], c.expires)
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

// normalProxy_ is the mixin for http[1-3]Proxy and hweb[1-2]Proxy.
type normalProxy_ struct {
	// Mixins
	Handlet_
	// Assocs
	stage   *Stage  // current stage
	app     *App    // the app to which the proxy belongs
	backend Backend // if works as forward proxy, this is nil
	cacher  Cacher  // the cacher which is used by this proxy
	// States
	hostname            []byte      // ...
	colonPort           []byte      // ...
	viaName             []byte      // ...
	isForward           bool        // reverse if false
	bufferClientContent bool        // buffer client content into tempFile?
	bufferServerContent bool        // buffer server content into tempFile?
	addRequestHeaders   [][2][]byte // headers appended to client request
	delRequestHeaders   [][]byte    // client request headers to delete
	addResponseHeaders  [][2][]byte // headers appended to server response
	delResponseHeaders  [][]byte    // server response headers to delete
}

func (h *normalProxy_) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}

func (h *normalProxy_) onConfigure() {
	// proxyMode
	if v, ok := h.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok && (mode == "forward" || mode == "reverse") {
			h.isForward = mode == "forward"
		} else {
			UseExitln("invalid proxyMode")
		}
	}

	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				h.backend = backend
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else if !h.isForward {
		UseExitln("toBackend is required for reverse proxy")
	}
	if h.isForward && !h.app.isDefault {
		UseExitln("forward proxy can be bound to default app only")
	}

	// withCacher
	if v, ok := h.Find("withCacher"); ok {
		if name, ok := v.String(); ok && name != "" {
			if cacher := h.stage.Cacher(name); cacher == nil {
				UseExitf("unknown cacher: '%s'\n", name)
			} else {
				h.cacher = cacher
			}
		} else {
			UseExitln("invalid withCacher")
		}
	}

	// hostname
	h.ConfigureBytes("hostname", &h.hostname, nil, nil)
	// colonPort
	h.ConfigureBytes("colonPort", &h.colonPort, nil, nil)
	// viaName
	h.ConfigureBytes("viaName", &h.viaName, nil, bytesGorox) // via: gorox
	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
	// delRequestHeaders
	h.ConfigureBytesList("delRequestHeaders", &h.delRequestHeaders, nil, [][]byte{})
	// delResponseHeaders
	h.ConfigureBytesList("delResponseHeaders", &h.delResponseHeaders, nil, [][]byte{})
}
func (h *normalProxy_) onPrepare() {
	// Currently nothing.
}

func (h *normalProxy_) IsProxy() bool { return true }
func (h *normalProxy_) IsCache() bool { return h.cacher != nil }

// Socket is the interface for *http[1-3]Socket.
type Socket interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
}

// serverSocket_ is the mixin for http[1-3]Socket.
type serverSocket_ struct {
	// Assocs
	shell Socket // the concrete Socket
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *serverSocket_) onUse() {
}
func (s *serverSocket_) onEnd() {
}

// socketProxy_ is the mixin for sock[1-3]Proxy.
type socketProxy_ struct {
	// Mixins
	Socklet_
	// Assocs
	stage   *Stage  // current stage
	app     *App    // the app to which the proxy belongs
	backend Backend // if works as forward proxy, this is nil
	// States
	isForward bool // reverse if false
}

func (s *socketProxy_) onCreate(name string, stage *Stage, app *App) {
	s.MakeComp(name)
	s.stage = stage
	s.app = app
}

func (s *socketProxy_) onConfigure() {
	// proxyMode
	if v, ok := s.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok && (mode == "forward" || mode == "reverse") {
			s.isForward = mode == "forward"
		} else {
			UseExitln("invalid proxyMode")
		}
	}
	// toBackend
	if v, ok := s.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := s.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else {
				s.backend = backend
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else if !s.isForward {
		UseExitln("toBackend is required for reverse proxy")
	}
	if s.isForward && !s.app.isDefault {
		UseExitln("forward proxy can be bound to default app only")
	}
}
func (s *socketProxy_) onPrepare() {
}

func (s *socketProxy_) IsProxy() bool { return true }
