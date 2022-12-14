// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General HTTP server implementation.

package internal

import (
	"bytes"
	"crypto/tls"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// httpServer is the interface for *httpxServer and *http3Server.
type httpServer interface {
	Server
	streamHolder

	linkApp(app *App)
	findApp(hostname []byte) *App
	linkSvc(svc *Svc)
	findSvc(hostname []byte) *Svc
}

// httpServer_ is a mixin for httpxServer and http3Server.
type httpServer_ struct {
	// Mixins
	Server_
	streamHolder_
	// Assocs
	gates      []httpGate
	defaultApp *App // fallback app
	// States
	exactApps    []*hostnameTo[*App] // like: ("example.com")
	suffixApps   []*hostnameTo[*App] // like: ("*.example.com")
	prefixApps   []*hostnameTo[*App] // like: ("www.example.*")
	exactSvcs    []*hostnameTo[*Svc] // like: ("example.com")
	suffixSvcs   []*hostnameTo[*Svc] // like: ("*.example.com")
	prefixSvcs   []*hostnameTo[*Svc] // like: ("www.example.*")
	hrpcMode     bool                // works as hrpc server and dispatches to svcs instead of apps?
	enableTCPTun bool                // allow CONNECT method?
	enableUDPTun bool                // allow upgrade: connect-udp?
}

func (s *httpServer_) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)
}

func (s *httpServer_) onConfigure(shell Component) {
	s.Server_.OnConfigure()
	s.streamHolder_.onConfigure(shell, 0)
	// hrpcMode
	s.ConfigureBool("hrpcMode", &s.hrpcMode, false)
	// enableTCPTun
	s.ConfigureBool("enableTCPTun", &s.enableTCPTun, false)
	// enableUDPTun
	s.ConfigureBool("enableUDPTun", &s.enableUDPTun, false)
}
func (s *httpServer_) onPrepare(shell Component) {
	s.Server_.OnPrepare()
	s.streamHolder_.onPrepare(shell)
}

func (s *httpServer_) linkApp(app *App) {
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
	app.linkServer(s.shell.(httpServer))
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
func (s *httpServer_) findApp(hostname []byte) *App {
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

func (s *httpServer_) linkSvc(svc *Svc) {
	svc.linkHRPC(s.shell.(httpServer))
	// TODO: use hash table?
	for _, hostname := range svc.exactHostnames {
		s.exactSvcs = append(s.exactSvcs, &hostnameTo[*Svc]{hostname, svc})
	}
	// TODO: use radix trie?
	for _, hostname := range svc.suffixHostnames {
		s.suffixSvcs = append(s.suffixSvcs, &hostnameTo[*Svc]{hostname, svc})
	}
	// TODO: use radix trie?
	for _, hostname := range svc.prefixHostnames {
		s.prefixSvcs = append(s.prefixSvcs, &hostnameTo[*Svc]{hostname, svc})
	}
}
func (s *httpServer_) findSvc(hostname []byte) *Svc {
	// TODO: use hash table?
	for _, exactMap := range s.exactSvcs {
		if bytes.Equal(hostname, exactMap.hostname) {
			return exactMap.target
		}
	}
	// TODO: use radix trie?
	for _, suffixMap := range s.suffixSvcs {
		if bytes.HasSuffix(hostname, suffixMap.hostname) {
			return suffixMap.target
		}
	}
	// TODO: use radix trie?
	for _, prefixMap := range s.prefixSvcs {
		if bytes.HasPrefix(hostname, prefixMap.hostname) {
			return prefixMap.target
		}
	}
	return nil
}

// httpGate is the interface for *httpxGate and *http3Gate.
type httpGate interface {
	Gate
	onConnectionClosed()
}

// httpGate_ is the mixin for httpxGate and http3Gate.
type httpGate_ struct {
	// Mixins
	Gate_
}

func (g *httpGate_) onConnectionClosed() {
	g.DecConns()
	g.SubDone()
}

// httpConn is the interface for *http[1-3]Conn.
type httpConn interface {
	serve() // goroutine
	getServer() httpServer
	isBroken() bool
	markBroken()
	makeTempName(p []byte, stamp int64) (from int, edge int) // small enough to be placed in tinyBuffer() of stream
}

// httpConn_ is the mixin for http[1-3]Conn.
type httpConn_ struct {
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id     int64      // the conn id
	server httpServer // the server to which the conn belongs
	gate   httpGate   // the gate to which the conn belongs
	// Conn states (zeros)
	lastRead    time.Time    // deadline of last read operation
	lastWrite   time.Time    // deadline of last write operation
	counter     atomic.Int64 // together with id, used to generate a random number as uploaded file's path
	usedStreams atomic.Int32 // num of streams served
	broken      atomic.Bool  // is conn broken?
}

func (c *httpConn_) onGet(id int64, server httpServer, gate httpGate) {
	c.id = id
	c.server = server
	c.gate = gate
}
func (c *httpConn_) onPut() {
	c.server = nil
	c.gate = nil
	c.lastRead = time.Time{}
	c.lastWrite = time.Time{}
	c.counter.Store(0)
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *httpConn_) getServer() httpServer { return c.server }
func (c *httpConn_) getGate() httpGate     { return c.gate }

func (c *httpConn_) isBroken() bool { return c.broken.Load() }
func (c *httpConn_) markBroken()    { c.broken.Store(true) }

func (c *httpConn_) makeTempName(p []byte, stamp int64) (from int, edge int) {
	return makeTempName(p, int64(c.server.Stage().ID()), c.id, stamp, c.counter.Add(1))
}

// httpStream_ is the mixin for http[1-3]Stream.
type httpStream_ struct {
	// Mixins
	stream_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *httpStream_) execTCPTun() {
	// TODO
}
func (s *httpStream_) execUDPTun() {
	// TODO
}
func (s *httpStream_) execSocket() {
	// TODO
}

// Request is the server-side HTTP request and is the interface for *http[1-3]Request.
type Request interface {
	PeerAddr() net.Addr
	App() *App
	Svc() *Svc

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

	QueryString() string // including '?' if query string exists, otherwise empty
	Q(name string) string
	Qstr(name string, defaultValue string) string
	Qint(name string, defaultValue int) int
	Query(name string) (value string, ok bool)
	QueryList(name string) (list []string, ok bool)
	Queries() (queries [][2]string)
	HasQuery(name string) bool
	AddQuery(name string, value string) bool
	DelQuery(name string) (deleted bool)

	H(name string) string
	Hstr(name string, defaultValue string) string
	Hint(name string, defaultValue int) int
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
	Cstr(name string, defaultValue string) string
	Cint(name string, defaultValue int) int
	Cookie(name string) (value string, ok bool)
	CookieList(name string) (list []string, ok bool)
	Cookies() (cookies [][2]string)
	HasCookies() bool
	HasCookie(name string) bool
	AddCookie(name string, value string) bool
	DelCookie(name string) (deleted bool)

	HasContent() bool
	SetMaxRecvTimeout(timeout time.Duration) // to defend against slowloris attack
	Content() string
	HoldContent() any

	F(name string) string
	Fstr(name string, defaultValue string) string
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

	HasTrailers() bool
	T(name string) string
	Tstr(name string, defaultValue string) string
	Tint(name string, defaultValue int) int
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
	UnsafeAuthority() []byte // hostname[:port]
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
	getPathInfo() os.FileInfo
	unsafeAbsPath() []byte
	makeAbsPath()
	applyHeader(header *pair) bool
	walkCookies(fn func(hash uint16, name []byte, value []byte) bool) bool
	delHopHeaders()
	unsetHost()
	walkHeaders(fn func(hash uint16, name []byte, value []byte) bool) bool
	walkTrailers(fn func(hash uint16, name []byte, value []byte) bool) bool
	recvContent(retain bool) any
	readContent() (p []byte, err error)
	delHopTrailers()
	applyTrailer(trailer *pair) bool
	arrayCopy(p []byte) bool
	getSaveContentFilesDir() string
	hookReviser(reviser Reviser)
	unsafeVariable(index int16) []byte
}

// httpRequest_ is the mixin for http[1-3]Request.
type httpRequest_ struct {
	// Mixins
	httpInMessage_
	// Stream states (buffers)
	stockUploads [2]Upload // for r.uploads. 96B
	// Stream states (controlled)
	ranges [2]span // parsed range fields. at most two range fields are allowed. controlled by r.nRanges
	// Stream states (non-zeros)
	uploads []Upload // decoded uploads -> r.array (for metadata) and temp files in local file system. [<r.stockUploads>/(make=16/128)]
	// Stream states (zeros)
	path          []byte      // decoded path. only a reference. refers to r.array or region if rewrited, so can't be a text
	absPath       []byte      // app.webRoot + r.UnsafePath(). if app.webRoot is not set then this is nil. set when dispatching to handlets. only a reference
	pathInfo      os.FileInfo // cached result of os.Stat(r.absPath) if r.absPath is not nil
	app           *App        // target app of this request. set before processing stream
	svc           *Svc        // target svc of this request. set before processing stream
	formWindow    []byte      // a window used when reading and parsing content as multipart/form-data. [<none>/r.content/4K/16K/64K1]
	httpRequest0_             // all values must be zero by default in this struct!
}
type httpRequest0_ struct { // for fast reset, entirely
	gotInput         bool     // got some input from client? for request timeout handling
	pathInfoGot      bool     // is r.pathInfo got?
	schemeCode       uint8    // SchemeHTTP, SchemeHTTPS
	targetForm       int8     // http request-target form. see httpTargetXXX
	methodCode       uint32   // known method code. 0: unknown method
	method           text     // raw method -> r.input
	authority        text     // raw hostname[:port] -> r.input
	hostname         text     // raw hostname (without :port) -> r.input
	colonPort        text     // raw colon port (:port, with ':') -> r.input
	uri              text     // raw uri (raw path & raw query string) -> r.input
	encodedPath      text     // raw path -> r.input
	queryString      text     // raw query string (with '?') -> r.input
	queries          zone     // decoded queries -> r.array
	cookies          zone     // raw cookies ->r.input|r.array. temporarily used when checking cookie headers, set after cookie is parsed
	forms            zone     // decoded forms -> r.array
	asteriskOptions  bool     // OPTIONS *?
	nRanges          int8     // num of ranges
	boundary         text     // boundary param of "multipart/form-data" if exists -> r.input
	ifRangeTime      int64    // parsed unix timestamp of if-range if is http-date format
	ifModifiedTime   int64    // parsed unix timestamp of if-modified-since
	ifUnmodifiedTime int64    // parsed unix timestamp of if-unmodified-since
	ifMatch          int8     // -1: if-match *, 0: no if-match field, >0: number of if-match: 1#entity-tag
	ifNoneMatch      int8     // -1: if-none-match *, 0: no if-none-match field, >0: number of if-none-match: 1#entity-tag
	ifMatches        zone     // the zone of if-match in r.primes
	ifNoneMatches    zone     // the zone of if-none-match in r.primes
	expectContinue   bool     // expect: 100-continue?
	acceptTrailers   bool     // does client accept trailers? i.e. te: trailers, gzip
	cacheControl     struct { // the cache-control info
		noCache      bool  // no-cache directive in cache-control
		noStore      bool  // no-store directive in cache-control
		noTransform  bool  // no-transform directive in cache-control
		onlyIfCached bool  // only-if-cached directive in cache-control
		maxAge       int32 // max-age directive in cache-control
		maxStale     int32 // max-stale directive in cache-control
		minFresh     int32 // min-fresh directive in cache-control
	}
	indexes struct { // indexes of some selected headers, for fast accessing
		host              uint8 // host header ->r.input
		userAgent         uint8 // user-agent header ->r.input
		ifRange           uint8 // if-range header ->r.input
		ifModifiedSince   uint8 // if-modified-since header ->r.input
		ifUnmodifiedSince uint8 // if-unmodified-since header ->r.input
	}
	revisers     [32]uint8 // reviser ids which will apply on this request. indexed by reviser order
	hasRevisers  bool      // are there any revisers hooked on this request?
	formReceived bool      // if content is a form, is it received?
	formKind     int8      // deducted type of form. 0:not form. see formXXX
	formEdge     int32     // edge position of the filled content in r.formWindow
	pFieldName   text      // raw field name. used during receiving and parsing multipart form in case of sliding r.formWindow
	sizeConsumed int64     // bytes of consumed content when consuming received TempFile. used by, for example, _recvMultipartForm.
}

func (r *httpRequest_) onUse() { // for non-zeros
	r.httpInMessage_.onUse(false)

	r.uploads = r.stockUploads[0:0:cap(r.stockUploads)] // use append()
}
func (r *httpRequest_) onEnd() { // for zeros
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
	r.svc = nil
	r.formWindow = nil // if r.formWindow is fetched from pool, it's put into pool at return. so just set as nil
	r.httpRequest0_ = httpRequest0_{}

	r.httpInMessage_.onEnd()
}

func (r *httpRequest_) App() *App { return r.app }
func (r *httpRequest_) Svc() *Svc { return r.svc }

func (r *httpRequest_) SchemeCode() uint8    { return r.schemeCode }
func (r *httpRequest_) Scheme() string       { return httpSchemeStrings[r.schemeCode] }
func (r *httpRequest_) UnsafeScheme() []byte { return httpSchemeByteses[r.schemeCode] }
func (r *httpRequest_) IsHTTP() bool         { return r.schemeCode == SchemeHTTP }
func (r *httpRequest_) IsHTTPS() bool        { return r.schemeCode == SchemeHTTPS }

func (r *httpRequest_) MethodCode() uint32   { return r.methodCode }
func (r *httpRequest_) Method() string       { return string(r.UnsafeMethod()) }
func (r *httpRequest_) UnsafeMethod() []byte { return r.input[r.method.from:r.method.edge] }
func (r *httpRequest_) IsGET() bool          { return r.methodCode == MethodGET }
func (r *httpRequest_) IsPOST() bool         { return r.methodCode == MethodPOST }
func (r *httpRequest_) IsPUT() bool          { return r.methodCode == MethodPUT }
func (r *httpRequest_) IsDELETE() bool       { return r.methodCode == MethodDELETE }
func (r *httpRequest_) recognizeMethod(method []byte, hash uint16) {
	if m := httpMethodTable[httpMethodFind(hash)]; m.hash == hash && bytes.Equal(httpMethodBytes[m.from:m.edge], method) {
		r.methodCode = m.code
	}
}

func (r *httpRequest_) IsAsteriskOptions() bool { return r.asteriskOptions }
func (r *httpRequest_) IsAbsoluteForm() bool    { return r.targetForm == httpTargetAbsolute }

func (r *httpRequest_) Authority() string       { return string(r.UnsafeAuthority()) }
func (r *httpRequest_) UnsafeAuthority() []byte { return r.input[r.authority.from:r.authority.edge] }
func (r *httpRequest_) Hostname() string        { return string(r.UnsafeHostname()) }
func (r *httpRequest_) UnsafeHostname() []byte  { return r.input[r.hostname.from:r.hostname.edge] }
func (r *httpRequest_) ColonPort() string {
	if r.colonPort.notEmpty() {
		return string(r.input[r.colonPort.from:r.colonPort.edge])
	}
	if r.schemeCode == SchemeHTTPS {
		return httpStringColonPort443
	} else {
		return httpStringColonPort80
	}
}
func (r *httpRequest_) UnsafeColonPort() []byte {
	if r.colonPort.notEmpty() {
		return r.input[r.colonPort.from:r.colonPort.edge]
	}
	if r.schemeCode == SchemeHTTPS {
		return httpBytesColonPort443
	} else {
		return httpBytesColonPort80
	}
}

func (r *httpRequest_) URI() string {
	if r.uri.notEmpty() {
		return string(r.input[r.uri.from:r.uri.edge])
	} else { // use "/"
		return httpStringSlash
	}
}
func (r *httpRequest_) UnsafeURI() []byte {
	if r.uri.notEmpty() {
		return r.input[r.uri.from:r.uri.edge]
	} else { // use "/"
		return httpBytesSlash
	}
}
func (r *httpRequest_) EncodedPath() string {
	if r.encodedPath.notEmpty() {
		return string(r.input[r.encodedPath.from:r.encodedPath.edge])
	} else { // use "/"
		return httpStringSlash
	}
}
func (r *httpRequest_) UnsafeEncodedPath() []byte {
	if r.encodedPath.notEmpty() {
		return r.input[r.encodedPath.from:r.encodedPath.edge]
	} else { // use "/"
		return httpBytesSlash
	}
}
func (r *httpRequest_) Path() string {
	if len(r.path) != 0 {
		return string(r.path)
	} else { // use "/"
		return httpStringSlash
	}
}
func (r *httpRequest_) UnsafePath() []byte {
	if len(r.path) != 0 {
		return r.path
	} else { // use "/"
		return httpBytesSlash
	}
}
func (r *httpRequest_) cleanPath() {
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
func (r *httpRequest_) unsafeAbsPath() []byte {
	return r.absPath
}
func (r *httpRequest_) makeAbsPath() {
	if r.app.webRoot == "" { // if app's webRoot is empty, r.absPath is not used either. so it's safe to do nothing
		return
	}
	webRoot := r.app.webRoot
	r.absPath = r.UnsafeMake(len(webRoot) + len(r.UnsafePath()))
	n := copy(r.absPath, webRoot)
	copy(r.absPath[n:], r.UnsafePath())
}
func (r *httpRequest_) getPathInfo() os.FileInfo {
	if !r.pathInfoGot {
		r.pathInfoGot = true
		if pathInfo, err := os.Stat(risky.WeakString(r.absPath)); err == nil {
			r.pathInfo = pathInfo
		}
	}
	return r.pathInfo
}

func (r *httpRequest_) QueryString() string {
	return string(r.UnsafeQueryString())
}
func (r *httpRequest_) UnsafeQueryString() []byte {
	return r.input[r.queryString.from:r.queryString.edge]
}
func (r *httpRequest_) addQuery(query *pair) bool {
	if edge, ok := r.addPrime(query); ok {
		r.queries.edge = edge
		return true
	} else {
		r.headResult, r.headReason = StatusURITooLong, "too many queries"
		return false
	}
}
func (r *httpRequest_) Q(name string) string {
	value, _ := r.Query(name)
	return value
}
func (r *httpRequest_) Qstr(name string, defaultValue string) string {
	if value, ok := r.Query(name); ok {
		return value
	}
	return defaultValue
}
func (r *httpRequest_) Qint(name string, defaultValue int) int {
	if value, ok := r.Query(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *httpRequest_) Query(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.queries, extraKindQuery)
	return string(v), ok
}
func (r *httpRequest_) UnsafeQuery(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.queries, extraKindQuery)
}
func (r *httpRequest_) QueryList(name string) (list []string, ok bool) {
	return r.getPairList(name, 0, r.queries, extraKindQuery)
}
func (r *httpRequest_) Queries() (queries [][2]string) {
	return r.getPairs(r.queries, extraKindQuery)
}
func (r *httpRequest_) HasQuery(name string) bool {
	_, ok := r.getPair(name, 0, r.queries, extraKindQuery)
	return ok
}
func (r *httpRequest_) AddQuery(name string, value string) bool {
	return r.addExtra(name, value, extraKindQuery)
}
func (r *httpRequest_) DelQuery(name string) (deleted bool) {
	return r.delPair(name, 0, r.queries, extraKindQuery)
}

func (r *httpRequest_) applyHeader(header *pair) bool {
	headerName := header.nameAt(r.input)
	if h := &httpRequestMultipleHeaderTable[httpRequestMultipleHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(httpRequestMultipleHeaderNames[h.from:h.edge], headerName) {
		if header.value.isEmpty() && h.must {
			r.headResult, r.headReason = StatusBadRequest, "empty value detected for field value format 1#(value)"
			return false
		}
		from := r.headers.edge
		if !r.addMultipleHeader(header, h.must) {
			// r.headResult is set.
			return false
		}
		if h.check != nil && !h.check(r, from, r.headers.edge) {
			// r.headResult is set.
			return false
		}
	} else { // single-value request header
		if !r.addHeader(header) {
			// r.headResult is set.
			return false
		}
		if h := &httpRequestCriticalHeaderTable[httpRequestCriticalHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(httpRequestCriticalHeaderNames[h.from:h.edge], headerName) {
			if h.check != nil && !h.check(r, header, r.headers.edge-1) {
				// r.headResult is set.
				return false
			}
		}
	}
	return true
}

var ( // perfect hash table for request multiple headers
	httpRequestMultipleHeaderNames = []byte("accept accept-charset accept-encoding accept-language cache-control connection content-encoding content-language forwarded if-match if-none-match pragma te trailer transfer-encoding upgrade via")
	httpRequestMultipleHeaderTable = [17]struct {
		hash  uint16
		from  uint8
		edge  uint8
		must  bool // true if 1#, false if #
		check func(*httpRequest_, uint8, uint8) bool
	}{
		0:  {httpHashAccept, 0, 6, false, nil},                                                // Accept = #( media-range [ accept-params ] )
		1:  {httpHashForwarded, 113, 122, true, nil},                                          // Forwarded = 1#forwarded-element
		2:  {httpHashUpgrade, 182, 189, true, (*httpRequest_).checkUpgrade},                   // Upgrade = 1#protocol
		3:  {httpHashCacheControl, 54, 67, true, (*httpRequest_).checkCacheControl},           // Cache-Control = 1#cache-directive
		4:  {httpHashAcceptLanguage, 38, 53, true, nil},                                       // Accept-Language = 1#( language-range [ weight ] )
		5:  {httpHashTE, 153, 155, false, (*httpRequest_).checkTE},                            // TE = #t-codings
		6:  {httpHashContentEncoding, 79, 95, true, (*httpRequest_).checkContentEncoding},     // Content-Encoding = 1#content-coding
		7:  {httpHashAcceptEncoding, 22, 37, false, (*httpRequest_).checkAcceptEncoding},      // Accept-Encoding = #( codings [ weight ] )
		8:  {httpHashVia, 190, 193, true, nil},                                                // Via = 1#( received-protocol RWS received-by [ RWS comment ] )
		9:  {httpHashContentLanguage, 96, 112, true, nil},                                     // Content-Language = 1#language-tag
		10: {httpHashConnection, 68, 78, true, (*httpRequest_).checkConnection},               // Connection = 1#connection-option
		11: {httpHashPragma, 146, 152, true, nil},                                             // Pragma = 1#pragma-directive
		12: {httpHashTransferEncoding, 164, 181, true, (*httpRequest_).checkTransferEncoding}, // Transfer-Encoding = 1#transfer-coding
		13: {httpHashTrailer, 156, 163, true, nil},                                            // Trailer = 1#field-name
		14: {httpHashAcceptCharset, 7, 21, true, nil},                                         // Accept-Charset = 1#( ( charset / "*" ) [ weight ] )
		15: {httpHashIfMatch, 123, 131, false, (*httpRequest_).checkIfMatch},                  // If-Match = "*" / #entity-tag
		16: {httpHashIfNoneMatch, 132, 145, false, (*httpRequest_).checkIfNoneMatch},          // If-None-Match = "*" / #entity-tag
	}
	httpRequestMultipleHeaderFind = func(hash uint16) int { return (48924603 / int(hash)) % 17 }
)

func (r *httpRequest_) checkCacheControl(from uint8, edge uint8) bool {
	// Cache-Control   = 1#cache-directive
	// cache-directive = token [ "=" ( token / quoted-string ) ]
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *httpRequest_) checkIfMatch(from uint8, edge uint8) bool {
	// If-Match = "*" / #entity-tag
	return r._checkMatch(from, edge, &r.ifMatches, &r.ifMatch)
}
func (r *httpRequest_) checkIfNoneMatch(from uint8, edge uint8) bool {
	// If-None-Match = "*" / #entity-tag
	return r._checkMatch(from, edge, &r.ifNoneMatches, &r.ifNoneMatch)
}
func (r *httpRequest_) checkTE(from uint8, edge uint8) bool {
	// TE        = #t-codings
	// t-codings = "trailers" / ( transfer-coding [ t-ranking ] )
	// t-ranking = OWS ";" OWS "q=" rank
	for i := from; i < edge; i++ {
		value := r.primes[i].valueAt(r.input)
		bytesToLower(value)
		if bytes.Equal(value, httpBytesTrailers) {
			r.acceptTrailers = true
		} else if r.versionCode > Version1_1 {
			r.headResult, r.headReason = StatusBadRequest, "te codings other than trailers are not allowed in http/2 and http/3"
			return false
		}
	}
	return true
}
func (r *httpRequest_) checkUpgrade(from uint8, edge uint8) bool {
	if r.versionCode == Version2 || r.versionCode == Version3 {
		r.headResult, r.headReason = StatusBadRequest, "upgrade is only supported in http/1.1"
		return false
	}
	if r.methodCode == MethodCONNECT {
		// TODO: confirm this
		return true
	}
	if r.versionCode == Version1_1 {
		// Upgrade          = 1#protocol
		// protocol         = protocol-name ["/" protocol-version]
		// protocol-name    = token
		// protocol-version = token
		for i := from; i < edge; i++ {
			value := r.primes[i].valueAt(r.input)
			bytesToLower(value)
			if bytes.Equal(value, httpBytesWebSocket) {
				r.upgradeSocket = true
			} else {
				// Unknown protocol. Ignored. We don't support "Upgrade: h2c" either.
			}
		}
	} else {
		// RFC 7230 (section 6.7):
		// A server MUST ignore an Upgrade header field that is received in an HTTP/1.0 request.
		for i := from; i < edge; i++ {
			r.delPrimeAt(i) // we delete it.
		}
	}
	return true
}
func (r *httpRequest_) _checkMatch(from uint8, edge uint8, matches *zone, match *int8) bool {
	if matches.isEmpty() {
		matches.from = from
	}
	matches.edge = edge
	for i := from; i < edge; i++ {
		header := &r.primes[i]
		value := header.valueAt(r.input)
		nMatch := *match // -1:*, 0:nonexist, >0:num
		if len(value) == 1 && value[0] == '*' {
			if nMatch != 0 {
				r.headResult, r.headReason = StatusBadRequest, "mix using of * and entity-tag"
				return false
			}
			*match = -1 // *
		} else { // entity-tag = [ weak ] opaque-tag
			// opaque-tag = DQUOTE *etagc DQUOTE
			if nMatch == -1 { // *
				r.headResult, r.headReason = StatusBadRequest, "mix using of entity-tag and *"
				return false
			}
			if nMatch > 63 {
				r.headResult, r.headReason = StatusBadRequest, "too many entity-tag"
				return false
			}
			// *match is 0 by default
			*match++
			if size := len(value); size >= 4 && value[0] == 'W' && value[1] == '/' && value[2] == '"' && value[size-1] == '"' { // W/"..."
				header.setWeakETag(true)
				header.value.from += 3
				header.value.edge--
			} else { // strong etag
				header.setWeakETag(false)
				if size >= 2 && value[0] == '"' && value[size-1] == '"' { // "..."
					header.value.from++
					header.value.edge--
				}
			}
		}
	}
	return true
}

var ( // perfect hash table for request critical headers
	httpRequestCriticalHeaderNames = []byte("content-length content-type cookie expect host if-modified-since if-range if-unmodified-since range user-agent") // authorization? proxy-authorization?
	httpRequestCriticalHeaderTable = [10]struct {
		hash  uint16
		from  uint8
		edge  uint8
		check func(*httpRequest_, *pair, uint8) bool
	}{
		0: {httpHashContentType, 15, 27, (*httpRequest_).checkContentType},
		1: {httpHashRange, 94, 99, (*httpRequest_).checkRange},
		2: {httpHashIfModifiedSince, 47, 64, (*httpRequest_).checkIfModifiedSince},
		3: {httpHashIfUnmodifiedSince, 74, 93, (*httpRequest_).checkIfUnmodifiedSince},
		4: {httpHashContentLength, 0, 14, (*httpRequest_).checkContentLength},
		5: {httpHashIfRange, 65, 73, (*httpRequest_).checkIfRange},
		6: {httpHashHost, 42, 46, (*httpRequest_).checkHost},
		7: {httpHashUserAgent, 100, 110, (*httpRequest_).checkUserAgent},
		8: {httpHashCookie, 28, 34, (*httpRequest_).checkCookie},
		9: {httpHashExpect, 35, 41, (*httpRequest_).checkExpect},
	}
	httpRequestCriticalHeaderFind = func(hash uint16) int { return (252525 / int(hash)) % 10 }
)

func (r *httpRequest_) checkAuthorization(header *pair, index uint8) bool {
	// TODO
	return true
}
func (r *httpRequest_) checkProxyAuthorization(header *pair, index uint8) bool {
	// TODO
	return true
}
func (r *httpRequest_) checkCookie(header *pair, index uint8) bool {
	// cookie-header = "Cookie:" OWS cookie-string OWS
	if header.value.isEmpty() {
		r.headResult, r.headReason = StatusBadRequest, "empty cookie"
		return false
	}
	if index == 255 {
		r.headResult, r.headReason = StatusBadRequest, "too many pairs"
		return false
	}
	// HTTP/2 and HTTP/3 allows multiple cookie headers, so we have to mark all the cookie headers.
	if r.cookies.isEmpty() {
		r.cookies.from = index
	}
	// And we can't inject cookies into headers, so we postpone cookie parsing after the request head is entirely received.
	r.cookies.edge = index + 1 // so only mark the edge
	return true
}
func (r *httpRequest_) checkExpect(header *pair, index uint8) bool {
	// Expect = "100-continue"
	value := header.valueAt(r.input)
	bytesToLower(value) // the Expect field-value is case-insensitive.
	if bytes.Equal(value, httpBytes100Continue) {
		if r.versionCode == Version1_0 {
			// RFC 7231 (section 5.1.1):
			// A server that receives a 100-continue expectation in an HTTP/1.0 request MUST ignore that expectation.
			r.delPrimeAt(index) // since HTTP/1.0 doesn't support 1xx status codes, we delete the expect.
		} else {
			r.expectContinue = true
		}
		return true
	} else {
		// RFC 7231 (section 5.1.1):
		// A server that receives an Expect field-value other than 100-continue
		// MAY respond with a 417 (Expectation Failed) status code to indicate
		// that the unexpected expectation cannot be met.
		r.headResult, r.headReason = StatusExpectationFailed, "only 100-continue is allowed in expect"
		return false
	}
}
func (r *httpRequest_) checkHost(header *pair, index uint8) bool {
	// Host = host [ ":" port ]
	// RFC 7230 (section 5.4): A server MUST respond with a 400 (Bad Request) status code to any
	// HTTP/1.1 request message that lacks a Host header field and to any request message that
	// contains more than one Host header field or a Host header field with an invalid field-value.
	if r.indexes.host != 0 {
		r.headResult, r.headReason = StatusBadRequest, "duplicate host header"
		return false
	}
	value := header.value
	if value.notEmpty() {
		// RFC 7230 (section 2.7.3.  http and https URI Normalization and Comparison):
		// The scheme and host are case-insensitive and normally provided in lowercase;
		// all other components are compared in a case-sensitive manner.
		bytesToLower(r.input[value.from:value.edge])
		if !r.parseAuthority(value.from, value.edge, r.authority.isEmpty()) {
			r.headResult, r.headReason = StatusBadRequest, "bad host value"
			return false
		}
	}
	r.indexes.host = index
	return true
}
func (r *httpRequest_) checkIfModifiedSince(header *pair, index uint8) bool {
	// If-Modified-Since = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.ifModifiedSince, &r.ifModifiedTime)
}
func (r *httpRequest_) checkIfRange(header *pair, index uint8) bool {
	// If-Range = entity-tag / HTTP-date
	if r.indexes.ifRange != 0 {
		r.headResult, r.headReason = StatusBadRequest, "duplicated if-range"
		return false
	}
	if modTime, ok := clockParseHTTPDate(header.valueAt(r.input)); ok {
		r.ifRangeTime = modTime
	}
	r.indexes.ifRange = index
	return true
}
func (r *httpRequest_) checkIfUnmodifiedSince(header *pair, index uint8) bool {
	// If-Unmodified-Since = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.ifUnmodifiedSince, &r.ifUnmodifiedTime)
}
func (r *httpRequest_) checkRange(header *pair, index uint8) bool {
	if r.methodCode != MethodGET {
		r.delPrimeAt(index)
		return true
	}
	if r.nRanges > 0 {
		r.headResult, r.headReason = StatusBadRequest, "duplicated range"
		return false
	}
	// Range        = range-unit "=" range-set
	// range-set    = 1#range-spec
	// range-spec   = int-range / suffix-range
	// int-range    = first-pos "-" [ last-pos ]
	// suffix-range = "-" suffix-length
	rangeSet := header.valueAt(r.input)
	nPrefix := len(httpBytesBytesEqual) // bytes=
	if !bytes.Equal(rangeSet[0:nPrefix], httpBytesBytesEqual) {
		r.headResult, r.headReason = StatusBadRequest, "unsupported range unit"
		return false
	}
	rangeSet = rangeSet[nPrefix:]
	if len(rangeSet) == 0 {
		r.headResult, r.headReason = StatusBadRequest, "empty range-set"
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
	r.headResult, r.headReason = StatusBadRequest, "invalid range"
	return false
}
func (r *httpRequest_) checkUserAgent(header *pair, index uint8) bool {
	if r.indexes.userAgent == 0 {
		r.indexes.userAgent = index
		return true
	} else {
		r.headResult, r.headReason = StatusBadRequest, "duplicated user-agent"
		return false
	}
}
func (r *httpRequest_) _addRange(from int64, last int64) bool {
	if r.nRanges == int8(cap(r.ranges)) {
		r.headResult, r.headReason = StatusBadRequest, "too many ranges"
		return false
	}
	r.ranges[r.nRanges] = span{from, last}
	r.nRanges++
	return true
}

func (r *httpRequest_) parseAuthority(from int32, edge int32, save bool) bool {
	if save {
		r.authority.set(from, edge)
	}
	// authority = host [ ":" port ]
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
func (r *httpRequest_) parseParams(p []byte, from int32, edge int32, paras []nava) (int, bool) {
	// param-string = *( OWS ";" OWS param-pair )
	// param-pair   = token "=" param-value
	// param-value  = *param-octet / ( DQUOTE *param-octet DQUOTE )
	// param-octet  = ?
	back, fore := from, from
	nAdd := 0
	for {
		nSemicolon := 0
		for fore < edge {
			if b := p[fore]; b == ';' {
				nSemicolon++
				fore++
			} else if b == ' ' || b == '\t' {
				fore++
			} else {
				break
			}
		}
		if fore == edge || nSemicolon != 1 {
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

func (r *httpRequest_) AcceptTrailers() bool { return r.acceptTrailers }
func (r *httpRequest_) UserAgent() string {
	return string(r.UnsafeUserAgent())
}
func (r *httpRequest_) UnsafeUserAgent() []byte {
	if r.indexes.userAgent == 0 {
		return nil
	}
	return r.primes[r.indexes.userAgent].valueAt(r.input)
}

func (r *httpRequest_) parseCookie(cookieString text) bool { // cookie: xxx
	// cookie-header = "Cookie:" OWS cookie-string OWS
	// cookie-string = cookie-pair *( ";" SP cookie-pair )
	// cookie-pair = token "=" cookie-value
	// cookie-value = *cookie-octet / ( DQUOTE *cookie-octet DQUOTE )
	// cookie-octet = %x21 / %x23-2B / %x2D-3A / %x3C-5B / %x5D-7E
	// exclude these: %x22=`"`  %2C=`,`  %3B=`;`  %5C=`\`
	var (
		state  = 0
		cookie pair // TODO: confirm not escape
	)
	cookie.setPlace(pairPlaceInput) // all received cookies are in r.input
	cookie.nameFrom = cookieString.from
	for p := cookieString.from; p < cookieString.edge; p++ {
		b := r.input[p]
		switch state {
		case 0: // expecting '=' to get cookie-name
			if b == '=' {
				if size := p - cookie.nameFrom; size > 0 && size <= 255 {
					cookie.nameSize = uint8(size)
				} else {
					r.headResult, r.headReason = StatusBadRequest, "cookie name out of range"
					return false
				}
				cookie.value.from = p + 1
				state = 1
			} else if httpTchar[b] != 0 {
				cookie.hash += uint16(b)
			} else {
				r.headResult, r.headReason = StatusBadRequest, "invalid cookie name"
				return false
			}
		case 1: // DQUOTE or not?
			if b == '"' {
				cookie.value.from++
				state = 3
				continue
			}
			state = 2
			fallthrough
		case 2: // *cookie-octet, expecting ';'
			if b == ';' {
				cookie.value.edge = p
				if !r.addCookie(&cookie) {
					return false
				}
				state = 5
			} else if b < 0x21 || b == '"' || b == ',' || b == '\\' || b > 0x7e {
				r.headResult, r.headReason = StatusBadRequest, "invalid cookie value"
				return false
			}
		case 3: // (DQUOTE *cookie-octet DQUOTE), expecting '"'
			if b == '"' {
				cookie.value.edge = p
				if !r.addCookie(&cookie) {
					return false
				}
				state = 4
			} else if b < 0x20 || b == ';' || b == '\\' || b > 0x7e { // ` ` and `,` are allowed here!
				r.headResult, r.headReason = StatusBadRequest, "invalid cookie value"
				return false
			}
		case 4: // expecting ';'
			if b != ';' {
				r.headResult, r.headReason = StatusBadRequest, "invalid cookie separator"
				return false
			}
			state = 5
		case 5: // expecting SP
			if b != ' ' {
				r.headResult, r.headReason = StatusBadRequest, "invalid cookie SP"
				return false
			}
			cookie.hash = 0
			cookie.nameFrom = p + 1
			state = 0
		}
	}
	if state == 2 { // ';' not found
		cookie.value.edge = cookieString.edge
		if !r.addCookie(&cookie) {
			return false
		}
	} else if state == 4 { // ';' not found
		if !r.addCookie(&cookie) {
			return false
		}
	} else {
		r.headResult, r.headReason = StatusBadRequest, "invalid cookie string"
		return false
	}
	return true
}
func (r *httpRequest_) addCookie(cookie *pair) bool { // cookie: xxx
	if edge, ok := r.addPrime(cookie); ok {
		r.cookies.edge = edge
		return true
	} else {
		r.headResult = StatusRequestHeaderFieldsTooLarge
		return false
	}
}
func (r *httpRequest_) C(name string) string {
	value, _ := r.Cookie(name)
	return value
}
func (r *httpRequest_) Cstr(name string, defaultValue string) string {
	if value, ok := r.Cookie(name); ok {
		return value
	}
	return defaultValue
}
func (r *httpRequest_) Cint(name string, defaultValue int) int {
	if value, ok := r.Cookie(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *httpRequest_) Cookie(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.cookies, extraKindCookie)
	return string(v), ok
}
func (r *httpRequest_) UnsafeCookie(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.cookies, extraKindCookie)
}
func (r *httpRequest_) CookieList(name string) (list []string, ok bool) {
	return r.getPairList(name, 0, r.cookies, extraKindCookie)
}
func (r *httpRequest_) Cookies() (cookies [][2]string) {
	return r.getPairs(r.cookies, extraKindCookie)
}
func (r *httpRequest_) HasCookies() bool {
	return r.hasPairs(r.cookies, extraKindCookie)
}
func (r *httpRequest_) HasCookie(name string) bool {
	_, ok := r.getPair(name, 0, r.cookies, extraKindCookie)
	return ok
}
func (r *httpRequest_) AddCookie(name string, value string) bool {
	return r.addExtra(name, value, extraKindCookie)
}
func (r *httpRequest_) DelCookie(name string) (deleted bool) {
	return r.delPair(name, 0, r.cookies, extraKindCookie)
}
func (r *httpRequest_) walkCookies(fn func(hash uint16, name []byte, value []byte) bool) bool {
	return r.forPairs(r.cookies, extraKindCookie, fn)
}

func (r *httpRequest_) TestConditions(modTime int64, etag []byte, asOrigin bool) (status int16, pass bool) { // to test preconditons intentionally
	// Get etag without ""
	if n := len(etag); n >= 2 && etag[0] == '"' && etag[n-1] == '"' {
		etag = etag[1 : n-1]
	}
	// See RFC 9110 (section 13.2.2).
	if asOrigin { // proxies ignore these tests.
		if r.ifMatch != 0 && !r.testIfMatch(etag) {
			return StatusPreconditionFailed, false
		}
		if r.ifMatch == 0 && r.indexes.ifUnmodifiedSince != 0 && !r.testIfUnmodifiedSince(modTime) {
			return StatusPreconditionFailed, false
		}
	}
	getOrHead := r.methodCode&(MethodGET|MethodHEAD) != 0
	if r.ifNoneMatch != 0 && !r.testIfNoneMatch(etag) {
		if getOrHead {
			return StatusNotModified, false
		} else {
			return StatusPreconditionFailed, false
		}
	}
	if getOrHead && r.ifNoneMatch == 0 && r.indexes.ifModifiedSince != 0 && !r.testIfModifiedSince(modTime) {
		return StatusNotModified, false
	}
	return StatusOK, true
}
func (r *httpRequest_) testIfMatch(etag []byte) (pass bool) {
	if r.ifMatch == -1 { // *
		return true
	}
	for i := r.ifMatches.from; i < r.ifMatches.edge; i++ {
		header := &r.primes[i]
		if header.hash != httpHashIfMatch || !header.nameEqualBytes(r.input, httpBytesIfMatch) {
			continue
		}
		if !header.isWeakETag() && bytes.Equal(header.valueAt(r.input), etag) {
			return true
		}
	}
	return false
}
func (r *httpRequest_) testIfNoneMatch(etag []byte) (pass bool) {
	if r.ifNoneMatch == -1 { // *
		return false
	}
	for i := r.ifNoneMatches.from; i < r.ifNoneMatches.edge; i++ {
		header := &r.primes[i]
		if header.hash != httpHashIfNoneMatch || !header.nameEqualBytes(r.input, httpBytesIfNoneMatch) {
			continue
		}
		if bytes.Equal(header.valueAt(r.input), etag) {
			return false
		}
	}
	return true
}
func (r *httpRequest_) testIfModifiedSince(modTime int64) (pass bool) {
	return modTime > r.ifModifiedTime
}
func (r *httpRequest_) testIfUnmodifiedSince(modTime int64) (pass bool) {
	return modTime <= r.ifUnmodifiedTime
}

func (r *httpRequest_) TestIfRanges(modTime int64, etag []byte, asOrigin bool) (pass bool) {
	if r.methodCode == MethodGET && r.nRanges > 0 && r.indexes.ifRange != 0 {
		if (r.ifRangeTime == 0 && r.testIfRangeETag(etag)) || (r.ifRangeTime != 0 && r.testIfRangeTime(modTime)) {
			return true // StatusPartialContent
		}
	}
	return false // StatusOK
}
func (r *httpRequest_) testIfRangeETag(etag []byte) (pass bool) {
	ifRange := &r.primes[r.indexes.ifRange]
	return !ifRange.isWeakETag() && bytes.Equal(ifRange.valueAt(r.input), etag)
}
func (r *httpRequest_) testIfRangeTime(modTime int64) (pass bool) {
	return r.ifRangeTime == modTime
}

func (r *httpRequest_) checkHead() bool {
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
			r.headResult, r.headReason = StatusBadRequest, "MUST send a Host header field in all HTTP/1.1 request messages"
			return false
		}
		if r.keepAlive == -1 { // no connection header
			r.keepAlive = 1 // default is keep-alive for HTTP/1.1
		}
	}

	// Resolve r.contentSize
	if r.transferChunked { // there is a transfer-encoding: chunked
		if r.contentSize != -1 { // there is a content-length: nnn
			// RFC 7230 (section 3.3.3):
			// If a message is received with both a Transfer-Encoding and a
			// Content-Length header field, the Transfer-Encoding overrides the
			// Content-Length.  Such a message might indicate an attempt to
			// perform request smuggling (Section 9.5) or response splitting
			// (Section 9.4) and ought to be handled as an error.  A sender MUST
			// remove the received Content-Length field prior to forwarding such
			// a message downstream.

			// We treat this as an error.
			r.headResult, r.headReason = StatusBadRequest, "transfer-encoding conflits with content-length"
			return false
		}
		r.contentSize = -2 // mark as chunked. use -2 to check chunked content from now on
	} else if r.versionCode >= Version2 && r.contentSize == -1 {
		// TODO: if there is no content, HTTP/2 and HTTP/3 will mark END_STREAM in headers frame.
		r.contentSize = -2 // if there is no content-length in HTTP/2 or HTTP/3, we treat it as chunked
	}

	if r.upgradeSocket && (r.methodCode != MethodGET || r.versionCode == Version1_0 || r.contentSize != -1) {
		// RFC 6455 (section 4.1):
		// The method of the request MUST be GET, and the HTTP version MUST be at least 1.1.
		r.headResult, r.headReason = StatusMethodNotAllowed, "websocket only supports GET method and HTTP version >= 1.1, without content"
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
			r.delHeader(httpBytesIfMatch, httpHashIfMatch)
			r.ifMatch = 0
		}
		if r.ifNoneMatch != 0 {
			r.delHeader(httpBytesIfNoneMatch, httpHashIfNoneMatch)
			r.ifNoneMatch = 0
		}
		if r.indexes.ifModifiedSince != 0 {
			r.delPrimeAt(r.indexes.ifModifiedSince)
			r.indexes.ifModifiedSince = 0
		}
		if r.indexes.ifUnmodifiedSince != 0 {
			r.delPrimeAt(r.indexes.ifUnmodifiedSince)
			r.indexes.ifUnmodifiedSince = 0
		}
		if r.indexes.ifRange != 0 {
			r.delPrimeAt(r.indexes.ifRange)
			r.indexes.ifRange = 0
		}
	} else {
		// RFC 9110 (section 13.1.3):
		// A recipient MUST ignore the If-Modified-Since header field if the
		// received field value is not a valid HTTP-date, the field value has
		// more than one member, or if the request method is neither GET nor HEAD.
		if r.indexes.ifModifiedSince != 0 && r.methodCode&(MethodGET|MethodHEAD) == 0 {
			r.delPrimeAt(r.indexes.ifModifiedSince) // we delete it.
			r.indexes.ifModifiedSince = 0
		}
		// A server MUST ignore an If-Range header field received in a request that does not contain a Range header field.
		if r.indexes.ifRange != 0 && r.nRanges == 0 {
			r.delPrimeAt(r.indexes.ifRange) // we delete it.
			r.indexes.ifRange = 0
		}
	}
	if r.contentSize == -1 { // no content
		if r.expectContinue { // expect is used to send large content.
			r.headResult, r.headReason = StatusBadRequest, "cannot use expect header without content"
			return false
		}
		if r.methodCode&(MethodPOST|MethodPUT) != 0 {
			r.headResult, r.headReason = StatusLengthRequired, "POST and PUT must contain a content"
			return false
		}
	} else { // content exists (counted or chunked)
		// Content is not allowed in some methods, according to RFC 7231.
		if r.methodCode&(MethodCONNECT|MethodTRACE) != 0 {
			r.headResult, r.headReason = StatusBadRequest, "content is not allowed in CONNECT and TRACE method"
			return false
		}
		if r.nContentCodings > 0 { // have content-encoding
			if r.nContentCodings > 1 || r.contentCodings[0] != httpCodingGzip {
				r.headResult, r.headReason = StatusUnsupportedMediaType, "currently only gzip content coding is supported in request"
				return false
			}
		}
		if r.iContentType == 0 {
			if r.methodCode == MethodOPTIONS {
				// RFC 7231 (section 4.3.7):
				// A client that generates an OPTIONS request containing a payload body
				// MUST send a valid Content-Type header field describing the
				// representation media type.
				r.headResult, r.headReason = StatusBadRequest, "OPTIONS with content but without a content-type"
				return false
			}
		} else {
			var (
				typeParams  text
				contentType []byte
			)
			vType := r.primes[r.iContentType].value
			if i := bytes.IndexByte(r.input[vType.from:vType.edge], ';'); i == -1 {
				typeParams.from = vType.edge
				typeParams.edge = vType.edge
				contentType = r.input[vType.from:vType.edge]
			} else {
				typeParams.from = vType.from + int32(i)
				typeParams.edge = typeParams.from  // too lazy to alloc a new variable. reuse typeParams.edge
				for typeParams.edge > vType.from { // skip OWS before ';'. for example: content-type: multipart/form-data ; boundary=xxx
					if b := r.input[typeParams.edge-1]; b == ' ' || b == '\t' {
						typeParams.edge--
					} else {
						break
					}
				}
				if typeParams.edge == vType.from { // TODO: if content-type is checked in r.checkContentType, we can remove this check
					r.headResult, r.headReason = StatusBadRequest, "content-type can't be an empty value"
					return false
				}
				contentType = r.input[vType.from:typeParams.edge]
				typeParams.edge = vType.edge
			}
			bytesToLower(contentType)
			if bytes.Equal(contentType, httpBytesURLEncodedForm) {
				r.formKind = httpFormURLEncoded
			} else if bytes.Equal(contentType, httpBytesMultipartForm) {
				paras := make([]nava, 1) // doesn't escape
				if _, ok := r.parseParams(r.input, typeParams.from, typeParams.edge, paras); !ok {
					r.headResult, r.headReason = StatusBadRequest, "invalid multipart/form-data params"
					return false
				}
				para := &paras[0]
				if bytes.Equal(r.input[para.name.from:para.name.edge], httpBytesBoundary) && para.value.notEmpty() && para.value.size() <= 70 && r.input[para.value.edge-1] != ' ' {
					// boundary := 0*69<bchars> bcharsnospace
					// bchars := bcharsnospace / " "
					// bcharsnospace := DIGIT / ALPHA / "'" / "(" / ")" / "+" / "_" / "," / "-" / "." / "/" / ":" / "=" / "?"
					r.boundary = para.value
					r.formKind = httpFormMultipart
				} else {
					r.headResult, r.headReason = StatusBadRequest, "bad boundary"
					return false
				}
			}
			if r.formKind != httpFormNotForm && r.nContentCodings > 0 {
				r.headResult, r.headReason = StatusUnsupportedMediaType, "a form with content coding is not supported yet"
				return false
			}
		}
	}
	if r.cookies.notEmpty() { // in HTTP/2 and HTTP/3, there can be multiple cookie fields.
		cookies := r.cookies                  // make a copy. r.cookies is changed as cookie name-value pairs below
		r.cookies.from = uint8(len(r.primes)) // r.cookies.edge is set in r.addCookie().
		for i := cookies.from; i < cookies.edge; i++ {
			cookie := &r.primes[i]
			if cookie.hash != httpHashCookie || !cookie.nameEqualBytes(r.input, httpBytesCookie) { // cookies may not be consecutive
				continue
			}
			if !r.parseCookie(cookie.value) {
				return false
			}
		}
	}

	return true
}

func (r *httpRequest_) unsetHost() { // used by proxies
	r.delPrimeAt(r.indexes.host) // zero safe
}

func (r *httpRequest_) parseHTMLForm() {
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
func (r *httpRequest_) _loadURLEncodedForm() { // into memory entirely
	r.loadContent()
	if r.stream.isBroken() {
		return
	}
	var (
		state = 2 // to be consistent with r.recvControl() in HTTP/1
		octet byte
		form  pair
	)
	form.setPlace(pairPlaceArray)
	form.nameFrom = r.arrayEdge
	for i := int64(0); i < r.sizeReceived; i++ { // TODO: use a better algorithm to improve performance
		b := r.contentBlob[i]
		switch state {
		case 2: // expecting '=' to get a name
			if b == '=' {
				if size := r.arrayEdge - form.nameFrom; size <= 255 {
					form.nameSize = uint8(size)
				} else {
					return
				}
				form.value.from = r.arrayEdge
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
				return
			}
		case 3: // expecting '&' to get a value
			if b == '&' {
				form.value.edge = r.arrayEdge
				if form.nameSize > 0 {
					r.addForm(&form)
				}
				form.hash = 0 // reset hash for next form
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
				return
			}
		default: // expecting HEXDIG
			half, ok := byteFromHex(b)
			if !ok {
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
			r.addForm(&form)
		}
	} else { // '=' not found, or incomplete pct-encoded
		// Do nothing, just ignore.
	}
}
func (r *httpRequest_) _recvMultipartForm() { // into memory or TempFile. see RFC 7578: https://www.rfc-editor.org/rfc/rfc7578.html
	var tempFile *os.File
	r.pBack, r.pFore = 0, 0
	r.sizeConsumed = r.sizeReceived
	if r.contentReceived { // (0, 64K1)
		// r.contentBlob is set, r.contentBlobKind == httpContentBlobInput. r.formWindow refers to the exact r.contentBlob.
		r.formWindow = r.contentBlob
		r.formEdge = int32(len(r.formWindow))
	} else { // content is not received
		r.contentReceived = true
		switch content := r.recvContent(true).(type) { // retain
		case []byte: // (0, 64K1]. case happens when counted content <= 64K1
			r.contentBlob = content
			r.contentBlobKind = httpContentBlobPool                                           // so r.contentBlob can be freed on end
			r.formWindow, r.formEdge = r.contentBlob[0:r.sizeReceived], int32(r.sizeReceived) // r.formWindow refers to the exact r.content.
		case TempFile: // [0, r.app.maxUploadContentSize]. case happens when counted content > 64K1, or content is chunked.
			tempFile = content.(*os.File)
			defer func() {
				tempFile.Close()
				os.Remove(tempFile.Name())
			}()
			if r.sizeReceived == 0 {
				// Chunked content can be empty.
				return
			}
			// We need a window to read and parse. An adaptive r.formWindow is used
			r.formWindow = GetNK(r.sizeReceived) // max size of r.formWindow is 64K1
			defer func() {
				PutNK(r.formWindow)
				r.formWindow = nil
			}()
			r.formEdge = 0     // no initial data, will fill below
			r.sizeConsumed = 0 // increases when we grow content
			if !r._growMultipartForm(tempFile) {
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
			if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
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
		if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
			return
		}
		// r.pFore is at fields of current part.
		var part struct { // current part
			valid  bool     // true if "name" param in "content-disposition" field is found
			isFile bool     // true if "filename" param in "content-disposition" field is found
			hash   uint16   //
			name   text     // to r.array. like: "avatar"
			base   text     // to r.array. like: "michael.jpg", or empty if part is not a file
			type_  text     // to r.array. like: "image/jpeg", or empty if part is not a file
			path   text     // to r.array. like: "/path/to/391384576", or empty if part is not a file
			osFile *os.File // if part is a file, this is used
			form   pair     // if part is a form, this is used
			upload Upload   // if part is a file, this is used. zeroed
		}
		part.form.setPlace(pairPlaceArray)
		for { // each field in current part
			// End of part fields?
			if b := r.formWindow[r.pFore]; b == '\r' {
				if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
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
				} else if b == ':' {
					break
				} else {
					r.stream.markBroken()
					return
				}
				if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
					return
				}
			}
			if r.pBack == r.pFore { // field-name cannot be empty
				r.stream.markBroken()
				return
			}
			r.pFieldName.set(r.pBack, r.pFore) // in case of sliding r.formWindow when r._growMultipartForm()
			// Skip ':'
			if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
				return
			}
			// Skip OWS before field value
			for r.formWindow[r.pFore] == ' ' || r.formWindow[r.pFore] == '\t' {
				if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
					return
				}
			}
			r.pBack = r.pFore // now r.formWindow is used for receiving field-value and onward. at this time we can still use r.pFieldName, no risk of sliding
			if fieldName := r.formWindow[r.pFieldName.from:r.pFieldName.edge]; bytes.Equal(fieldName, httpBytesContentDisposition) {
				// form-data; name="avatar"; filename="michael.jpg"
				for r.formWindow[r.pFore] != ';' {
					if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
						return
					}
				}
				if r.pBack == r.pFore || !bytes.Equal(r.formWindow[r.pBack:r.pFore], httpBytesFormData) {
					r.stream.markBroken()
					return
				}
				r.pBack = r.pFore // now r.formWindow is used for receiving params and onward
				for r.formWindow[r.pFore] != '\n' {
					if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
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
				paras := make([]nava, 2) // for name & filename. won't escape to heap
				n, ok := r.parseParams(r.formWindow, r.pBack, fore, paras)
				if !ok {
					r.stream.markBroken()
					return
				}
				for i := 0; i < n; i++ { // each para in field (; name="avatar"; filename="michael.jpg")
					para := &paras[i]
					if paraName := r.formWindow[para.name.from:para.name.edge]; bytes.Equal(paraName, httpBytesName) { // name="avatar"
						if n := para.value.size(); n == 0 || n > 255 {
							r.stream.markBroken()
							return
						}
						part.valid = true
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
					} else if bytes.Equal(paraName, httpBytesFilename) { // filename="michael.jpg"
						part.isFile = true
						if n := para.value.size(); n > 0 && n <= 255 {
							part.base.from = r.arrayEdge
							if !r.arrayCopy(r.formWindow[para.value.from:para.value.edge]) { // add "michael.jpg"
								r.stream.markBroken()
								return
							}
							part.base.edge = r.arrayEdge
							part.path.from = r.arrayEdge
							if !r.arrayCopy(risky.ConstBytes(r.app.saveContentFilesDir)) { // add "/path/to/"
								r.stream.markBroken()
								return
							}
							tempName := r.stream.tinyBuffer() // buffer is enough for tempName
							from, edge := r.stream.makeTempName(tempName, r.recvTime.Unix())
							if !r.arrayCopy(tempName[from:edge]) { // add "391384576"
								r.stream.markBroken()
								return
							}
							// TODO: ensure pathSize <= 255
							part.path.edge = r.arrayEdge
						}
					} else {
						// Other parameters are invalid.
						r.stream.markBroken()
						return
					}
				}
			} else if bytes.Equal(fieldName, httpBytesContentType) {
				// image/jpeg
				for r.formWindow[r.pFore] != '\n' {
					if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
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
				if n := fore - r.pBack; n > 0 && n <= 255 {
					part.type_.from = r.arrayEdge
					if !r.arrayCopy(r.formWindow[r.pBack:fore]) { // add "image/jpeg"
						r.stream.markBroken()
						return
					}
					part.type_.edge = r.arrayEdge
				}
			} else { // other fields are ignored
				for r.formWindow[r.pFore] != '\n' {
					if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
						return
					}
				}
			}
			// Skip '\n' and goto next field or end of fields
			if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
				return
			}
		}
		if !part.valid { // no valid fields
			r.stream.markBroken()
			return
		}
		// Now all fields of the part are received. Skip end of fields and goto part data
		if r.pFore++; r.pFore == r.formEdge && !r._growMultipartForm(tempFile) {
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
		} else {
			part.form.hash = part.hash
			part.form.nameSize, part.form.nameFrom = uint8(part.name.size()), part.name.from
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
			if !r._growMultipartForm(tempFile) {
				return
			}
		}
	}
}
func (r *httpRequest_) _growMultipartForm(tempFile *os.File) bool { // caller needs more data.
	if r.sizeConsumed == r.sizeReceived || (r.formEdge == int32(len(r.formWindow)) && r.pBack == 0) {
		r.stream.markBroken()
		return false
	}
	if r.pBack > 0 { // have useless data. slide to start
		copy(r.formWindow, r.formWindow[r.pBack:r.formEdge])
		r.formEdge -= r.pBack
		r.pFore -= r.pBack
		r.pFieldName.sub(r.pBack) // for fields in multipart/form-data, not for trailers
		r.pBack = 0
	}
	if n, err := tempFile.Read(r.formWindow[r.formEdge:]); err == nil {
		r.formEdge += int32(n)
		r.sizeConsumed += int64(n)
		return true
	} else {
		r.stream.markBroken()
		return false
	}
}

func (r *httpRequest_) addForm(form *pair) {
	if edge, ok := r.addPrime(form); ok {
		r.forms.edge = edge
	}
	// Ignore too many forms
}
func (r *httpRequest_) F(name string) string {
	value, _ := r.Form(name)
	return value
}
func (r *httpRequest_) Fstr(name string, defaultValue string) string {
	if value, ok := r.Form(name); ok {
		return value
	}
	return defaultValue
}
func (r *httpRequest_) Fint(name string, defaultValue int) int {
	if value, ok := r.Form(name); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
func (r *httpRequest_) Form(name string) (value string, ok bool) {
	r.parseHTMLForm()
	v, ok := r.getPair(name, 0, r.forms, extraKindNoExtra)
	return string(v), ok
}
func (r *httpRequest_) UnsafeForm(name string) (value []byte, ok bool) {
	r.parseHTMLForm()
	return r.getPair(name, 0, r.forms, extraKindNoExtra)
}
func (r *httpRequest_) FormList(name string) (list []string, ok bool) {
	r.parseHTMLForm()
	return r.getPairList(name, 0, r.forms, extraKindNoExtra)
}
func (r *httpRequest_) Forms() (forms [][2]string) {
	r.parseHTMLForm()
	return r.getPairs(r.forms, extraKindNoExtra)
}
func (r *httpRequest_) HasForm(name string) bool {
	r.parseHTMLForm()
	_, ok := r.getPair(name, 0, r.forms, extraKindNoExtra)
	return ok
}

func (r *httpRequest_) addUpload(upload *Upload) {
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
func (r *httpRequest_) U(name string) *Upload {
	upload, _ := r.Upload(name)
	return upload
}
func (r *httpRequest_) Upload(name string) (upload *Upload, ok bool) {
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
func (r *httpRequest_) UploadList(name string) (list []*Upload, ok bool) {
	r.parseHTMLForm()
	if n := len(r.uploads); n > 0 && name != "" {
		hash := stringHash(name)
		for i := 0; i < n; i++ {
			if upload := &r.uploads[i]; upload.hash == hash && upload.nameEqualString(r.array, name) {
				upload.setMeta(r.array)
				list = append(list, upload)
			}
		}
		if len(list) > 0 {
			ok = true
		}
	}
	return
}
func (r *httpRequest_) Uploads() (uploads []*Upload) {
	r.parseHTMLForm()
	for i := 0; i < len(r.uploads); i++ {
		upload := &r.uploads[i]
		upload.setMeta(r.array)
		uploads = append(uploads, upload)
	}
	return uploads
}
func (r *httpRequest_) HasUpload(name string) bool {
	r.parseHTMLForm()
	_, ok := r.Upload(name)
	return ok
}

func (r *httpRequest_) HasContent() bool {
	return r.contentSize >= 0 || r.contentSize == -2 // -2 means chunked
}
func (r *httpRequest_) UnsafeContent() []byte {
	if r.formKind == httpFormMultipart { // loading multipart form into memory is not allowed!
		return nil
	}
	return r.unsafeContent()
}

func (r *httpRequest_) applyTrailer(trailer *pair) bool {
	r.addTrailer(trailer)
	// TODO: check trailer? Pseudo-header fields MUST NOT appear in a trailer section.
	return true
}

func (r *httpRequest_) arrayCopy(p []byte) bool {
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

func (r *httpRequest_) getSaveContentFilesDir() string {
	return r.app.saveContentFilesDir // must ends with '/'
}

func (r *httpRequest_) hookReviser(reviser Reviser) {
	r.hasRevisers = true
	r.revisers[reviser.Rank()] = reviser.ID() // revisers are placed to fixed position, by their ranks.
}

func (r *httpRequest_) unsafeVariable(index int16) []byte {
	return httpRequestVariables[index](r)
}

var httpRequestVariables = [...]func(*httpRequest_) []byte{ // keep sync with varCodes in config.go
	(*httpRequest_).UnsafeMethod,      // method
	(*httpRequest_).UnsafeScheme,      // scheme
	(*httpRequest_).UnsafeAuthority,   // authority
	(*httpRequest_).UnsafeHostname,    // hostname
	(*httpRequest_).UnsafeColonPort,   // colonPort
	(*httpRequest_).UnsafePath,        // path
	(*httpRequest_).UnsafeURI,         // uri
	(*httpRequest_).UnsafeEncodedPath, // encodedPath
	(*httpRequest_).UnsafeQueryString, // queryString
	(*httpRequest_).UnsafeContentType, // contentType
}

// Response is the server-side HTTP response and is the interface for *http[1-3]Response.
type Response interface {
	Request() Request

	SetStatus(status int16) error
	Status() int16

	MakeETagFrom(modTime int64, fileSize int64) ([]byte, bool) // with `""`
	SetLastModified(lastModified int64) bool
	AddHTTPSRedirection(authority string) bool
	AddHostnameRedirection(hostname string) bool
	AddDirectoryRedirection() bool

	SetCookie(setCookie *SetCookie) bool

	Header(name string) (value string, ok bool)
	HasHeader(name string) bool
	AddHeader(name string, value string) bool
	AddHeaderBytes(name string, value []byte) bool
	AddHeaderByBytes(name []byte, value string) bool
	AddHeaderBytesByBytes(name []byte, value []byte) bool
	DelHeader(name string) bool
	DelHeaderByBytes(name []byte) bool

	IsSent() bool
	SetMaxSendTimeout(timeout time.Duration) // to defend against slowloris attack

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
	AddTrailerBytes(name string, value []byte) bool
	AddTrailerByBytes(name []byte, value string) bool
	AddTrailerBytesByBytes(name []byte, value []byte) bool

	// Internal only
	header(name []byte) (value []byte, ok bool)
	hasHeader(name []byte) bool
	addHeader(name []byte, value []byte) bool
	delHeader(name []byte) bool
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

// httpResponse_ is the mixin for http[1-3]Response.
type httpResponse_ struct {
	// Mixins
	httpOutMessage_
	// Assocs
	request Request // *http[1-3]Request
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	status       int16 // 200, 302, 404, 500, ...
	lastModified int64 // -1: not set, -2: set through general api, >= 0: set unix timestamp in seconds
	// Stream states (zeros)
	app            *App // associated app
	svc            *Svc // associated svc
	httpResponse0_      // all values must be zero by default in this struct!
}
type httpResponse0_ struct { // for fast reset, entirely
	revisers      [32]uint8 // reviser ids which will apply on this response. indexed by reviser order
	hasRevisers   bool      // are there any revisers hooked on this response?
	oExpires      uint8     // ...
	oLastModified uint8     // ...
}

func (r *httpResponse_) onUse() { // for non-zeros
	r.httpOutMessage_.onUse(false)
	r.status = StatusOK
	r.lastModified = -1
}
func (r *httpResponse_) onEnd() { // for zeros
	r.app = nil
	r.svc = nil
	r.httpResponse0_ = httpResponse0_{}
	r.httpOutMessage_.onEnd()
}

func (r *httpResponse_) Request() Request { return r.request }

func (r *httpResponse_) SetStatus(status int16) error {
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
		return httpUnknownStatus
	}
}
func (r *httpResponse_) Status() int16 {
	return r.status
}

func (r *httpResponse_) MakeETagFrom(modTime int64, fileSize int64) ([]byte, bool) { // with ""
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
func (r *httpResponse_) SetLastModified(lastModified int64) bool {
	if lastModified < 0 {
		return false
	}
	if r.lastModified == -2 {
		r.shell.delHeaderAt(r.oLastModified)
		r.oLastModified = 0
	}
	r.lastModified = lastModified
	return true
}

var ( // perfect hash table for response crucial headers
	httpResponseCrucialHeaderNames = []byte("connection content-length content-type date expires last-modified server set-cookie transfer-encoding upgrade")
	httpResponseCrucialHeaderTable = [10]struct {
		hash uint16
		from uint8
		edge uint8
		fAdd func(*httpResponse_, []byte) (ok bool)
		fDel func(*httpResponse_) (deleted bool)
	}{
		0: {httpHashServer, 66, 72, nil, nil},
		1: {httpHashSetCookie, 73, 83, nil, nil},
		2: {httpHashUpgrade, 102, 109, nil, nil},
		3: {httpHashDate, 39, 43, (*httpResponse_).joinDate, (*httpResponse_).kickDate},
		4: {httpHashTransferEncoding, 84, 101, nil, nil},
		5: {httpHashConnection, 0, 10, nil, nil},
		6: {httpHashLastModified, 52, 65, (*httpResponse_).joinLastModified, (*httpResponse_).kickLastModified},
		7: {httpHashExpires, 44, 51, (*httpResponse_).joinExpires, (*httpResponse_).kickExpires},
		8: {httpHashContentLength, 11, 25, nil, nil},
		9: {httpHashContentType, 26, 38, (*httpResponse_).joinContentType, (*httpResponse_).kickContentType},
	}
	httpResponseCrucialHeaderFind = func(hash uint16) int { return (113100 / int(hash)) % 10 }
)

func (r *httpResponse_) joinHeader(hash uint16, name []byte, value []byte) bool {
	h := &httpResponseCrucialHeaderTable[httpResponseCrucialHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(httpResponseCrucialHeaderNames[h.from:h.edge], name) {
		if h.fAdd == nil {
			return true // pretend to be successful
		}
		return h.fAdd(r, value)
	}
	return r.shell.addHeader(name, value)
}
func (r *httpResponse_) kickHeader(hash uint16, name []byte) bool {
	h := &httpResponseCrucialHeaderTable[httpResponseCrucialHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(httpResponseCrucialHeaderNames[h.from:h.edge], name) {
		if h.fDel == nil {
			return true // pretend to be successful
		}
		return h.fDel(r)
	}
	return r.shell.delHeader(name)
}

func (r *httpResponse_) joinExpires(expires []byte) (ok bool) {
	// TODO
	return r.shell.addHeader(httpBytesExpires, expires)
}
func (r *httpResponse_) joinLastModified(lastModified []byte) (ok bool) {
	if r.lastModified == -2 {
		r.shell.delHeaderAt(r.oLastModified)
		r.oLastModified = 0
	} else { // >= 0 or -1
		r.lastModified = -2
	}
	if !r.shell.addHeader(httpBytesLastModified, lastModified) {
		return false
	}
	r.oLastModified = r.nHeaders - 1
	return true
}

func (r *httpResponse_) kickExpires() (deleted bool) {
	// TODO
	return true
}
func (r *httpResponse_) kickLastModified() (deleted bool) {
	if r.lastModified == -1 {
		return false
	}
	if r.lastModified == -2 {
		r.shell.delHeaderAt(r.oLastModified)
		r.oLastModified = 0
	}
	r.lastModified = -1
	return true
}

func (r *httpResponse_) SendBadRequest(content []byte) error { // 400
	return r.sendError(StatusBadRequest, content)
}
func (r *httpResponse_) SendForbidden(content []byte) error { // 403
	return r.sendError(StatusForbidden, content)
}
func (r *httpResponse_) SendNotFound(content []byte) error { // 404
	return r.sendError(StatusNotFound, content)
}
func (r *httpResponse_) SendMethodNotAllowed(allow string, content []byte) error { // 405
	r.AddHeaderByBytes(httpBytesAllow, allow)
	return r.sendError(StatusMethodNotAllowed, content)
}
func (r *httpResponse_) SendInternalServerError(content []byte) error { // 500
	return r.sendError(StatusInternalServerError, content)
}
func (r *httpResponse_) SendNotImplemented(content []byte) error { // 501
	return r.sendError(StatusNotImplemented, content)
}
func (r *httpResponse_) SendBadGateway(content []byte) error { // 502
	return r.sendError(StatusBadGateway, content)
}
func (r *httpResponse_) SendGatewayTimeout(content []byte) error { // 504
	return r.sendError(StatusGatewayTimeout, content)
}
func (r *httpResponse_) sendError(status int16, content []byte) error {
	if err := r.checkSend(); err != nil {
		return err
	}
	if err := r.SetStatus(status); err != nil {
		return err
	}
	if content == nil {
		content = httpErrorPages[status]
	}
	r.content.head.SetBlob(content)
	r.contentSize = int64(len(content))
	return r.shell.sendChain(r.content)
}
func (r *httpResponse_) send() error {
	curChain := r.content
	resp := r.shell.(Response)
	if r.hasRevisers {
		// Travel through revisers
		for _, id := range r.revisers { // revise headers
			if id == 0 { // reviser id is ensured to be > 0
				continue
			}
			reviser := r.app.reviserByID(id)
			reviser.BeforeSend(resp.Request(), resp)
		}
		for _, id := range r.revisers { // revise content
			if id == 0 {
				continue
			}
			reviser := r.app.reviserByID(id)
			newChain := reviser.OnOutput(resp.Request(), resp, curChain)
			if newChain != curChain { // chain has been replaced by reviser
				curChain.free()
				curChain = newChain
			}
		}
		// Because r.content chain may be altered/replaced by revisers, content size must be recalculated
		r.contentSize = 0
		for block := curChain.head; block != nil; block = block.next {
			r.contentSize += block.size
			if r.contentSize < 0 {
				return httpContentTooLarge
			}
		}
	}
	return resp.sendChain(curChain)
}

func (r *httpResponse_) checkPush() error {
	if r.stream.isBroken() {
		return httpWriteBroken
	}
	if r.isSent {
		return nil
	}
	if r.contentSize != -1 {
		return httpMixedContentMode
	}
	r.isSent = true
	r.contentSize = -2 // mark as chunked mode
	resp := r.shell.(Response)
	if r.hasRevisers {
		for _, id := range r.revisers {
			if id == 0 { // reviser id is ensured to be > 0
				continue
			}
			reviser := r.app.reviserByID(id)
			reviser.BeforePush(resp.Request(), resp)
		}
	}
	return resp.pushHeaders()
}
func (r *httpResponse_) push(chunk *Block) error {
	var curChain Chain
	curChain.PushTail(chunk)
	defer curChain.free()

	if r.stream.isBroken() {
		return httpWriteBroken
	}
	resp := r.shell.(Response)
	if r.hasRevisers {
		for _, id := range r.revisers {
			if id == 0 { // reviser id is ensured to be > 0
				continue
			}
			reviser := r.app.reviserByID(id)
			newChain := reviser.OnOutput(resp.Request(), resp, curChain)
			if newChain != curChain { // chain has be replaced by reviser
				curChain.free()
				curChain = newChain
			}
		}
	}
	return resp.pushChain(curChain)
}

func (r *httpResponse_) copyHead(resp response) bool { // used by proxies
	r.SetStatus(resp.Status())

	resp.delHopHeaders()

	// copy crucial headers (excluding set-cookie) from resp

	// copy remaining headers
	if !resp.walkHeaders(func(hash uint16, name []byte, value []byte) bool {
		if hash == httpHashSetCookie && bytes.Equal(name, httpBytesSetCookie) {
			return r.shell.addHeader(name, value)
		} else {
			return r.shell.joinHeader(hash, name, value)
		}
	}) {
		return false
	}

	return true
}
func (r *httpResponse_) pass(resp httpInMessage) error { // used by proxies
	return r.doPass(resp, r.hasRevisers)
}

func (r *httpResponse_) finishChunked() error {
	if r.stream.isBroken() {
		return httpWriteBroken
	}
	resp := r.shell.(Response)
	if r.hasRevisers {
		for _, id := range r.revisers {
			if id == 0 { // reviser id is ensured to be > 0
				continue
			}
			reviser := r.app.reviserByID(id)
			reviser.FinishPush(resp.Request(), resp)
		}
	}
	return resp.finalizeChunked()
}

func (r *httpResponse_) hookReviser(reviser Reviser) {
	r.hasRevisers = true
	r.revisers[reviser.Rank()] = reviser.ID() // revisers are placed to fixed position, by their ranks.
}

// SetCookie is a "set-cookie" sent to client.
type SetCookie struct {
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

func (c *SetCookie) Set(name string, value string) bool {
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

func (c *SetCookie) SetDomain(domain string) bool {
	// TODO: check domain
	c.domain = domain
	return true
}
func (c *SetCookie) SetPath(path string) bool {
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
func (c *SetCookie) SetExpires(expires time.Time) bool {
	if expires.Year() < 1601 {
		c.invalid = true
		return false
	}
	c.expires = expires
	return true
}
func (c *SetCookie) SetMaxAge(maxAge int64) { c.maxAge = maxAge }
func (c *SetCookie) SetSecure()             { c.secure = true }
func (c *SetCookie) SetHttpOnly()           { c.httpOnly = true }
func (c *SetCookie) SetSameSiteStrict()     { c.sameSite = "Strict" }
func (c *SetCookie) SetSameSiteLax()        { c.sameSite = "Lax" }
func (c *SetCookie) SetSameSiteNone()       { c.sameSite = "None" }

func (c *SetCookie) size() int {
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
func (c *SetCookie) writeTo(p []byte) int {
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

// Socket is the server-side WebSocket and is the interface for *http[1-3]Socket.
type Socket interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
}

// httpSocket_ is the mixin for http[1-3]Socket.
type httpSocket_ struct {
	// Assocs
	shell Socket // the concrete Socket
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *httpSocket_) onUse() {
}
func (s *httpSocket_) onEnd() {
}
