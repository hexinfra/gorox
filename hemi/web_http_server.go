// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// General HTTP server implementation. See RFC 9110.

package hemi

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func init() {
	RegisterHandlet("static", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(staticHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// HTTPServer
type HTTPServer interface { // for *http[1-3]Server
	// Imports
	Server
	contentSaver
	// Methods
	RecvTimeout() time.Duration // timeout to recv the whole message content
	SendTimeout() time.Duration // timeout to send the whole message
	MaxContentSize() int64
	MaxMemoryContentSize() int32
	MaxStreamsPerConn() int32

	bindApps()
	findApp(hostname []byte) *Webapp
}

// httpServer_ is the parent for http[1-3]Server.
type httpServer_[G Gate] struct {
	// Parent
	Server_[G]
	// Mixins
	_httpServend_
	// Assocs
	defaultApp *Webapp // default webapp if not found
	// States
	webapps      []string               // for what webapps
	exactApps    []*hostnameTo[*Webapp] // like: ("example.com")
	suffixApps   []*hostnameTo[*Webapp] // like: ("*.example.com")
	prefixApps   []*hostnameTo[*Webapp] // like: ("www.example.*")
	forceScheme  int8                   // scheme (http/https) that must be used
	adjustScheme bool                   // use https scheme for TLS and http scheme for others?
}

func (s *httpServer_[G]) onCreate(name string, stage *Stage) {
	s.Server_.OnCreate(name, stage)

	s.forceScheme = -1 // not forced
}

func (s *httpServer_[G]) onConfigure() {
	s.Server_.OnConfigure()
	s._httpServend_.onConfigure(s, 120*time.Second, 120*time.Second, 1000, TmpDir()+"/web/servers/"+s.name)

	// webapps
	s.ConfigureStringList("webapps", &s.webapps, nil, []string{})

	// forceScheme
	var scheme string
	s.ConfigureString("forceScheme", &scheme, func(value string) error {
		if value != "http" && value != "https" {
			return errors.New(".forceScheme has an invalid value")
		}
		return nil
	}, "")
	switch scheme {
	case "http":
		s.forceScheme = SchemeHTTP
	case "https":
		s.forceScheme = SchemeHTTPS
	}

	// adjustScheme
	s.ConfigureBool("adjustScheme", &s.adjustScheme, true)
}
func (s *httpServer_[G]) onPrepare() {
	s.Server_.OnPrepare()
	s._httpServend_.onPrepare(s)
}

func (s *httpServer_[G]) bindApps() {
	for _, appName := range s.webapps {
		webapp := s.stage.Webapp(appName)
		if webapp == nil {
			continue
		}
		if s.IsTLS() {
			if webapp.tlsCertificate == "" || webapp.tlsPrivateKey == "" {
				UseExitln("webapps that bound to tls server must have certificates and private keys")
			}
			certificate, err := tls.LoadX509KeyPair(webapp.tlsCertificate, webapp.tlsPrivateKey)
			if err != nil {
				UseExitln(err.Error())
			}
			if DebugLevel() >= 1 {
				Printf("adding certificate to %s\n", s.ColonPort())
			}
			s.tlsConfig.Certificates = append(s.tlsConfig.Certificates, certificate)
		}
		webapp.bindServer(s.shell.(HTTPServer))
		if webapp.isDefault {
			s.defaultApp = webapp
		}
		// TODO: use hash table?
		for _, hostname := range webapp.exactHostnames {
			s.exactApps = append(s.exactApps, &hostnameTo[*Webapp]{hostname, webapp})
		}
		// TODO: use radix trie?
		for _, hostname := range webapp.suffixHostnames {
			s.suffixApps = append(s.suffixApps, &hostnameTo[*Webapp]{hostname, webapp})
		}
		// TODO: use radix trie?
		for _, hostname := range webapp.prefixHostnames {
			s.prefixApps = append(s.prefixApps, &hostnameTo[*Webapp]{hostname, webapp})
		}
	}
}
func (s *httpServer_[G]) findApp(hostname []byte) *Webapp {
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
	return s.defaultApp // may be nil
}

// Request is the server-side http request.
type Request interface { // for *server[1-3]Request
	RemoteAddr() net.Addr
	Webapp() *Webapp

	IsAbsoluteForm() bool    // TODO: what about HTTP/2 and HTTP/3?
	IsAsteriskOptions() bool // OPTIONS *

	VersionCode() uint8
	IsHTTP1_0() bool
	IsHTTP1_1() bool
	IsHTTP1() bool
	IsHTTP2() bool
	IsHTTP3() bool
	Version() string // HTTP/1.0, HTTP/1.1, HTTP/2, HTTP/3
	UnsafeVersion() []byte

	SchemeCode() uint8 // SchemeHTTP, SchemeHTTPS
	IsHTTP() bool
	IsHTTPS() bool
	Scheme() string // http, https
	UnsafeScheme() []byte

	MethodCode() uint32
	IsGET() bool
	IsPOST() bool
	IsPUT() bool
	IsDELETE() bool
	Method() string // GET, POST, ...
	UnsafeMethod() []byte

	Authority() string       // hostname[:port]
	UnsafeAuthority() []byte // hostname[:port]
	Hostname() string        // hostname
	UnsafeHostname() []byte  // hostname
	ColonPort() string       // :port
	UnsafeColonPort() []byte // :port

	URI() string               // /encodedPath?queryString
	UnsafeURI() []byte         // /encodedPath?queryString
	Path() string              // /decodedPath
	UnsafePath() []byte        // /decodedPath
	EncodedPath() string       // /encodedPath
	UnsafeEncodedPath() []byte // /encodedPath
	QueryString() string       // including '?' if query string exists, otherwise empty
	UnsafeQueryString() []byte // including '?' if query string exists, otherwise empty

	HasQueries() bool
	AllQueries() (queries [][2]string)
	Q(name string) string
	Qstr(name string, defaultValue string) string
	Qint(name string, defaultValue int) int
	Query(name string) (value string, ok bool)
	UnsafeQuery(name string) (value []byte, ok bool)
	Queries(name string) (values []string, ok bool)
	HasQuery(name string) bool
	DelQuery(name string) (deleted bool)
	AddQuery(name string, value string) bool

	HasHeaders() bool
	AllHeaders() (headers [][2]string)
	H(name string) string
	Hstr(name string, defaultValue string) string
	Hint(name string, defaultValue int) int
	Header(name string) (value string, ok bool)
	UnsafeHeader(name string) (value []byte, ok bool)
	Headers(name string) (values []string, ok bool)
	HasHeader(name string) bool
	DelHeader(name string) (deleted bool)
	AddHeader(name string, value string) bool

	UserAgent() string
	UnsafeUserAgent() []byte

	ContentType() string
	UnsafeContentType() []byte

	ContentSize() int64
	UnsafeContentLength() []byte

	AcceptTrailers() bool

	EvalPreconditions(date int64, etag []byte, asOrigin bool) (status int16, normal bool)

	HasIfRange() bool
	EvalIfRange(date int64, etag []byte, asOrigin bool) (canRange bool)

	HasRanges() bool
	EvalRanges(size int64) []Range

	HasCookies() bool
	AllCookies() (cookies [][2]string)
	C(name string) string
	Cstr(name string, defaultValue string) string
	Cint(name string, defaultValue int) int
	Cookie(name string) (value string, ok bool)
	UnsafeCookie(name string) (value []byte, ok bool)
	Cookies(name string) (values []string, ok bool)
	HasCookie(name string) bool
	DelCookie(name string) (deleted bool)
	AddCookie(name string, value string) bool

	SetRecvTimeout(timeout time.Duration) // to defend against slowloris attack

	HasContent() bool // true if content exists
	IsVague() bool    // true if content exists and is not sized
	Content() string
	UnsafeContent() []byte

	HasForms() bool
	AllForms() (forms [][2]string)
	F(name string) string
	Fstr(name string, defaultValue string) string
	Fint(name string, defaultValue int) int
	Form(name string) (value string, ok bool)
	UnsafeForm(name string) (value []byte, ok bool)
	Forms(name string) (values []string, ok bool)
	HasForm(name string) bool
	AddForm(name string, value string) bool

	HasUpfiles() bool
	AllUpfiles() (upfiles []*Upfile)
	U(name string) *Upfile
	Upfile(name string) (upfile *Upfile, ok bool)
	Upfiles(name string) (upfiles []*Upfile, ok bool)
	HasUpfile(name string) bool

	HasTrailers() bool
	AllTrailers() (trailers [][2]string)
	T(name string) string
	Tstr(name string, defaultValue string) string
	Tint(name string, defaultValue int) int
	Trailer(name string) (value string, ok bool)
	UnsafeTrailer(name string) (value []byte, ok bool)
	Trailers(name string) (values []string, ok bool)
	HasTrailer(name string) bool
	DelTrailer(name string) (deleted bool)
	AddTrailer(name string, value string) bool

	UnsafeMake(size int) []byte

	// Internal only
	getPathInfo() os.FileInfo
	unsafeAbsPath() []byte
	makeAbsPath()
	delHopHeaders()
	delHopTrailers()
	forHeaders(callback func(header *pair, name []byte, value []byte) bool) bool
	forTrailers(callback func(trailer *pair, name []byte, value []byte) bool) bool
	forCookies(callback func(cookie *pair, name []byte, value []byte) bool) bool
	unsetHost()
	holdContent() any
	readContent() (p []byte, err error)
	examineTail() bool
	hookReviser(reviser Reviser)
	unsafeVariable(code int16, name string) (value []byte)
}

// serverRequest_ is the parent for server[1-3]Request.
type serverRequest_ struct { // incoming. needs parsing
	// Parent
	httpIn_ // incoming http message
	// Stream states (stocks)
	stockUpfiles [2]Upfile // for r.upfiles. 96B
	// Stream states (controlled)
	ranges [4]Range // parsed range fields. at most 4 range fields are allowed. controlled by r.nRanges
	// Stream states (non-zeros)
	upfiles []Upfile // decoded upfiles -> r.array (for metadata) and temp files in local file system. [<r.stockUpfiles>/(make=16/128)]
	// Stream states (zeros)
	webapp         *Webapp     // target webapp of this request. set before executing the stream
	path           []byte      // decoded path. only a reference. refers to r.array or region if rewrited, so can't be a span
	absPath        []byte      // webapp.webRoot + r.UnsafePath(). if webapp.webRoot is not set then this is nil. set when dispatching to handlets. only a reference
	pathInfo       os.FileInfo // cached result of os.Stat(r.absPath) if r.absPath is not nil
	formWindow     []byte      // a window used when reading and parsing content as multipart/form-data. [<none>/r.contentText/4K/16K]
	serverRequest0             // all values must be zero by default in this struct!
}
type serverRequest0 struct { // for fast reset, entirely
	gotInput        bool     // got some input from client? for request timeout handling
	targetForm      int8     // request-target form. see httpTargetXXX
	asteriskOptions bool     // true if method and uri is: OPTIONS *
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
	nRanges         int8     // num of ranges. controls r.ranges
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
		maxForwards        uint8 // max-forwards header ->r.input
		proxyAuthorization uint8 // proxy-authorization header ->r.input
		userAgent          uint8 // user-agent header ->r.input
	}
	zones struct { // zones (may not be continuous) of some selected headers, for fast accessing
		acceptLanguage zone
		expect         zone
		forwarded      zone
		ifMatch        zone // the zone of if-match in r.primes
		ifNoneMatch    zone // the zone of if-none-match in r.primes
		xForwardedFor  zone
		_              [4]byte // padding
	}
	unixTimes struct { // parsed unix times in seconds
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

func (r *serverRequest_) onUse(httpVersion uint8) { // for non-zeros
	const asResponse = false
	r.httpIn_.onUse(httpVersion, asResponse)

	r.upfiles = r.stockUpfiles[0:0:cap(r.stockUpfiles)] // use append()
}
func (r *serverRequest_) onEnd() { // for zeros
	for _, upfile := range r.upfiles {
		if upfile.isMoved() { // file was moved, don't remove it
			continue
		}
		var filePath string
		if upfile.metaSet() {
			filePath = upfile.Path()
		} else {
			filePath = WeakString(r.array[upfile.pathFrom : upfile.pathFrom+int32(upfile.pathSize)])
		}
		if err := os.Remove(filePath); err != nil {
			r.webapp.Logf("failed to remove uploaded file: %s, error: %s\n", filePath, err.Error())
		}
	}
	r.upfiles = nil

	r.webapp = nil
	r.path = nil
	r.absPath = nil
	r.pathInfo = nil
	r.formWindow = nil // if r.formWindow is fetched from pool, it's put into pool on return. so just set as nil
	r.serverRequest0 = serverRequest0{}

	r.httpIn_.onEnd()
}

func (r *serverRequest_) Webapp() *Webapp { return r.webapp }

func (r *serverRequest_) IsAbsoluteForm() bool    { return r.targetForm == httpTargetAbsolute }
func (r *serverRequest_) IsAsteriskOptions() bool { return r.asteriskOptions }

func (r *serverRequest_) SchemeCode() uint8    { return r.schemeCode }
func (r *serverRequest_) Scheme() string       { return webSchemeStrings[r.schemeCode] }
func (r *serverRequest_) UnsafeScheme() []byte { return webSchemeByteses[r.schemeCode] }
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

func (r *serverRequest_) Authority() string { return string(r.UnsafeAuthority()) }
func (r *serverRequest_) UnsafeAuthority() []byte {
	return r.input[r.authority.from:r.authority.edge]
}
func (r *serverRequest_) Hostname() string       { return string(r.UnsafeHostname()) }
func (r *serverRequest_) UnsafeHostname() []byte { return r.input[r.hostname.from:r.hostname.edge] }
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
	slashed := r.path[nPath-1] == '/'
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
		if slashed && pReal > 1 {
			r.path[pReal] = '/'
			pReal++
		}
		r.path = r.path[:pReal]
	}
}
func (r *serverRequest_) unsafeAbsPath() []byte { return r.absPath }
func (r *serverRequest_) makeAbsPath() {
	if r.webapp.webRoot == "" { // if webapp's webRoot is empty, r.absPath is not used either. so it's safe to do nothing
		return
	}
	webRoot := r.webapp.webRoot
	r.absPath = r.UnsafeMake(len(webRoot) + len(r.UnsafePath()))
	n := copy(r.absPath, webRoot)
	copy(r.absPath[n:], r.UnsafePath())
}
func (r *serverRequest_) getPathInfo() os.FileInfo {
	if !r.pathInfoGot {
		r.pathInfoGot = true
		if pathInfo, err := os.Stat(WeakString(r.absPath)); err == nil {
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
func (r *serverRequest_) HasQueries() bool { return r.hasPairs(r.queries, pairQuery) }
func (r *serverRequest_) AllQueries() (queries [][2]string) {
	return r.allPairs(r.queries, pairQuery)
}
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
	v, ok := r.getPair(name, 0, r.queries, pairQuery)
	return string(v), ok
}
func (r *serverRequest_) UnsafeQuery(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.queries, pairQuery)
}
func (r *serverRequest_) Queries(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.queries, pairQuery)
}
func (r *serverRequest_) HasQuery(name string) bool {
	_, ok := r.getPair(name, 0, r.queries, pairQuery)
	return ok
}
func (r *serverRequest_) DelQuery(name string) (deleted bool) {
	return r.delPair(name, 0, r.queries, pairQuery)
}
func (r *serverRequest_) AddQuery(name string, value string) bool { // as extra
	return r.addExtra(name, value, 0, pairQuery)
}

func (r *serverRequest_) examineHead() bool {
	for i := r.headers.from; i < r.headers.edge; i++ {
		if !r.applyHeader(i) {
			// r.headResult is set.
			return false
		}
	}
	if r.cookies.notEmpty() { // in HTTP/2 and HTTP/3, there can be multiple cookie fields.
		cookies := r.cookies // make a copy, as r.cookies will be changed as cookie pairs below
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
	if DebugLevel() >= 3 {
		Println("======primes======")
		for i := 0; i < len(r.primes); i++ {
			prime := &r.primes[i]
			prime.show(r._placeOf(prime))
		}
		Println("======extras======")
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
	switch r.httpVersion {
	case Version1_0:
		if r.keepAlive == -1 { // no connection header
			r.keepAlive = 0 // default is close for HTTP/1.0
		}
	case Version1_1:
		if r.keepAlive == -1 { // no connection header
			r.keepAlive = 1 // default is keep-alive for HTTP/1.1
		}
		if r.indexes.host == 0 {
			// RFC 7230 (section 5.4):
			// A client MUST send a Host header field in all HTTP/1.1 request messages.
			r.headResult, r.failReason = StatusBadRequest, "MUST send a Host header field in all HTTP/1.1 request messages"
			return false
		}
	default: // HTTP/2 and HTTP/3
		r.keepAlive = 1 // default is keep-alive for HTTP/2 and HTTP/3
		// TODO: Add other checks here
	}

	if !r.determineContentMode() {
		// r.headResult is set.
		return false
	}
	if r.contentSize > r.maxContentSize {
		r.headResult, r.failReason = StatusContentTooLarge, "content size exceeds server's limit"
		return false
	}

	if r.upgradeSocket {
		// RFC 6455 (section 4.1):
		// The method of the request MUST be GET, and the HTTP version MUST be at least 1.1.
		if r.methodCode != MethodGET || r.httpVersion == Version1_0 || r.contentSize != -1 {
			r.headResult, r.failReason = StatusMethodNotAllowed, "websocket only supports GET method and HTTP version >= 1.1, without content"
			return false
		}
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
	} else { // content exists (sized or vague)
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
		} else if !r._parseField(header, &sh.fdesc, r.input, true) { // fully
			r.headResult = StatusBadRequest
			return false
		}
		if !sh.check(r, header, index) {
			// r.headResult is set.
			return false
		}
	} else if mh := &serverRequestImportantHeaderTable[serverRequestImportantHeaderFind(header.hash)]; mh.hash == header.hash && bytes.Equal(mh.name, name) {
		extraFrom := uint8(len(r.extras))
		if !r._splitField(header, &mh.fdesc, r.input) {
			r.headResult = StatusBadRequest
			return false
		}
		if header.isCommaValue() { // has sub headers, check them
			if extraEdge := uint8(len(r.extras)); !mh.check(r, r.extras, extraFrom, extraEdge) {
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
	serverRequestSingletonHeaderTable = [13]struct {
		parse bool // need general parse or not
		fdesc      // allowQuote, allowEmpty, allowParam, hasComment
		check func(*serverRequest_, *pair, uint8) bool
	}{ // authorization content-length content-type cookie date host if-modified-since if-range if-unmodified-since max-forwards proxy-authorization range user-agent
		0:  {false, fdesc{hashContentLength, false, false, false, false, bytesContentLength}, (*serverRequest_).checkContentLength},
		1:  {false, fdesc{hashIfUnmodifiedSince, false, false, false, false, bytesIfUnmodifiedSince}, (*serverRequest_).checkIfUnmodifiedSince},
		2:  {false, fdesc{hashMaxForwards, false, false, false, false, bytesMaxForwards}, (*serverRequest_).checkMaxForwards},
		3:  {false, fdesc{hashUserAgent, false, false, false, true, bytesUserAgent}, (*serverRequest_).checkUserAgent},
		4:  {false, fdesc{hashIfRange, false, false, false, false, bytesIfRange}, (*serverRequest_).checkIfRange},
		5:  {false, fdesc{hashCookie, false, false, false, false, bytesCookie}, (*serverRequest_).checkCookie}, // `a=b; c=d; e=f` is cookie list, not parameters
		6:  {false, fdesc{hashProxyAuthorization, false, false, false, false, bytesProxyAuthorization}, (*serverRequest_).checkProxyAuthorization},
		7:  {false, fdesc{hashIfModifiedSince, false, false, false, false, bytesIfModifiedSince}, (*serverRequest_).checkIfModifiedSince},
		8:  {true, fdesc{hashContentType, false, false, true, false, bytesContentType}, (*serverRequest_).checkContentType},
		9:  {false, fdesc{hashDate, false, false, false, false, bytesDate}, (*serverRequest_).checkDate},
		10: {false, fdesc{hashAuthorization, false, false, false, false, bytesAuthorization}, (*serverRequest_).checkAuthorization},
		11: {false, fdesc{hashRange, false, false, false, false, bytesRange}, (*serverRequest_).checkRange},
		12: {false, fdesc{hashHost, false, false, false, false, bytesHost}, (*serverRequest_).checkHost},
	}
	serverRequestSingletonHeaderFind = func(hash uint16) int { return (811410 / int(hash)) % 13 }
)

func (r *serverRequest_) checkAuthorization(header *pair, index uint8) bool { // Authorization = auth-scheme [ 1*SP ( token68 / #auth-param ) ]
	// auth-scheme = token
	// token68     = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
	// auth-param  = token BWS "=" BWS ( token / quoted-string )
	// TODO
	if r.indexes.authorization != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated authorization header"
		return false
	}
	r.indexes.authorization = index
	return true
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
		r.headResult, r.failReason = StatusBadRequest, "duplicated if-range header"
		return false
	}
	if date, ok := clockParseHTTPDate(header.valueAt(r.input)); ok {
		r.unixTimes.ifRange = date
	}
	r.indexes.ifRange = index
	return true
}
func (r *serverRequest_) checkIfUnmodifiedSince(header *pair, index uint8) bool { // If-Unmodified-Since = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.ifUnmodifiedSince, &r.unixTimes.ifUnmodifiedSince)
}
func (r *serverRequest_) checkMaxForwards(header *pair, index uint8) bool { // Max-Forwards = Max-Forwards = 1*DIGIT
	if r.indexes.maxForwards != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated max-forwards header"
		return false
	}
	// TODO
	r.indexes.maxForwards = index
	return true
}
func (r *serverRequest_) checkProxyAuthorization(header *pair, index uint8) bool { // Proxy-Authorization = auth-scheme [ 1*SP ( token68 / #auth-param ) ]
	// auth-scheme = token
	// token68     = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
	// auth-param  = token BWS "=" BWS ( token / quoted-string )
	// TODO
	if r.indexes.proxyAuthorization != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated proxyAuthorization header"
		return false
	}
	r.indexes.proxyAuthorization = index
	return true
}
func (r *serverRequest_) checkRange(header *pair, index uint8) bool { // Range = ranges-specifier
	if r.methodCode != MethodGET {
		// A server MUST ignore a Range header field received with a request method that is unrecognized or for which range handling is not defined.
		// For this specification, GET is the only method for which range handling is defined.
		r._delPrime(index)
		return true
	}
	if r.nRanges > 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated range header"
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
	var rang Range // [from-last], inclusive, begins from 0
	state := 0
	for i, n := 0, len(rangeSet); i < n; i++ {
		b := rangeSet[i]
		switch state {
		case 0: // select int-range or suffix-range
			if b >= '0' && b <= '9' {
				rang.From = int64(b - '0')
				state = 1 // int-range
			} else if b == '-' {
				rang.From = -1
				rang.Last = 0
				state = 4 // suffix-range
			} else if b != ',' && b != ' ' {
				goto badRange
			}
		case 1: // in first-pos = 1*DIGIT
			for ; i < n; i++ {
				if b := rangeSet[i]; b >= '0' && b <= '9' {
					rang.From = rang.From*10 + int64(b-'0')
					if rang.From < 0 { // overflow
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
				rang.Last = int64(b - '0')
				state = 3 // first-pos "-" last-pos
			} else if b == ',' || b == ' ' { // got: first-pos "-"
				rang.Last = -1
				if !r._addRange(rang) {
					return false
				}
				state = 0 // select int-range or suffix-range
			} else {
				goto badRange
			}
		case 3: // in last-pos = 1*DIGIT
			for ; i < n; i++ {
				if b := rangeSet[i]; b >= '0' && b <= '9' {
					rang.Last = rang.Last*10 + int64(b-'0')
					if rang.Last < 0 { // overflow
						goto badRange
					}
				} else if b == ',' || b == ' ' { // got: first-pos "-" last-pos
					// An int-range is invalid if the last-pos value is present and less than the first-pos.
					if rang.From > rang.Last {
						goto badRange
					}
					if !r._addRange(rang) {
						return false
					}
					state = 0 // select int-range or suffix-range
					break
				} else {
					goto badRange
				}
			}
		case 4: // in suffix-length = 1*DIGIT
			for ; i < n; i++ {
				if b := rangeSet[i]; b >= '0' && b <= '9' {
					rang.Last = rang.Last*10 + int64(b-'0')
					if rang.Last < 0 { // overflow
						goto badRange
					}
				} else if b == ',' || b == ' ' { // got: "-" suffix-length
					if !r._addRange(rang) {
						return false
					}
					state = 0 // select int-range or suffix-range
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
		rang.Last = -1
	}
	if (state == 2 || state == 3 || state == 4) && !r._addRange(rang) {
		return false
	}
	return true
badRange:
	r.headResult, r.failReason = StatusBadRequest, "invalid range"
	return false
}
func (r *serverRequest_) _addRange(rang Range) bool {
	if r.nRanges == int8(cap(r.ranges)) {
		r.headResult, r.failReason = StatusBadRequest, "too many ranges"
		return false
	}
	r.ranges[r.nRanges] = rang
	r.nRanges++
	return true
}
func (r *serverRequest_) checkUserAgent(header *pair, index uint8) bool { // User-Agent = product *( RWS ( product / comment ) )
	if r.indexes.userAgent != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated user-agent header"
		return false
	}
	r.indexes.userAgent = index
	return true
}

var ( // perfect hash table for important request headers
	serverRequestImportantHeaderTable = [16]struct {
		fdesc // allowQuote, allowEmpty, allowParam, hasComment
		check func(*serverRequest_, []pair, uint8, uint8) bool
	}{ // accept-encoding accept-language cache-control connection content-encoding content-language expect forwarded if-match if-none-match te trailer transfer-encoding upgrade via x-forwarded-for
		0:  {fdesc{hashIfMatch, true, false, false, false, bytesIfMatch}, (*serverRequest_).checkIfMatch},
		1:  {fdesc{hashContentLanguage, false, false, false, false, bytesContentLanguage}, (*serverRequest_).checkContentLanguage},
		2:  {fdesc{hashVia, false, false, false, true, bytesVia}, (*serverRequest_).checkVia},
		3:  {fdesc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*serverRequest_).checkTransferEncoding}, // deliberately false
		4:  {fdesc{hashCacheControl, false, false, false, false, bytesCacheControl}, (*serverRequest_).checkCacheControl},
		5:  {fdesc{hashConnection, false, false, false, false, bytesConnection}, (*serverRequest_).checkConnection},
		6:  {fdesc{hashForwarded, false, false, false, false, bytesForwarded}, (*serverRequest_).checkForwarded}, // `for=192.0.2.60;proto=http;by=203.0.113.43` is not parameters
		7:  {fdesc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*serverRequest_).checkUpgrade},
		8:  {fdesc{hashXForwardedFor, false, false, false, false, bytesXForwardedFor}, (*serverRequest_).checkXForwardedFor},
		9:  {fdesc{hashExpect, false, false, true, false, bytesExpect}, (*serverRequest_).checkExpect},
		10: {fdesc{hashAcceptEncoding, false, true, true, false, bytesAcceptEncoding}, (*serverRequest_).checkAcceptEncoding},
		11: {fdesc{hashContentEncoding, false, false, false, false, bytesContentEncoding}, (*serverRequest_).checkContentEncoding},
		12: {fdesc{hashAcceptLanguage, false, false, true, false, bytesAcceptLanguage}, (*serverRequest_).checkAcceptLanguage},
		13: {fdesc{hashIfNoneMatch, true, false, false, false, bytesIfNoneMatch}, (*serverRequest_).checkIfNoneMatch},
		14: {fdesc{hashTE, false, false, true, false, bytesTE}, (*serverRequest_).checkTE},
		15: {fdesc{hashTrailer, false, false, false, false, bytesTrailer}, (*serverRequest_).checkTrailer},
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
	if DebugLevel() >= 2 {
		/*
			for i := from; i < edge; i++ {
				// NOTE: test pair.kind == pairHeader
				data := pairs[i].dataAt(r.input)
				Printf("lang=%s\n", string(data))
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
	if r.httpVersion >= Version1_1 {
		if r.zones.expect.isEmpty() {
			r.zones.expect.from = from
		}
		r.zones.expect.edge = edge
		for i := from; i < edge; i++ {
			pair := &pairs[i]
			if pair.kind != pairHeader {
				continue
			}
			data := pair.dataAt(r.input)
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
func (r *serverRequest_) checkTE(pairs []pair, from uint8, edge uint8) bool { // TE = #t-codings
	// t-codings = "trailers" / ( transfer-coding [ t-ranking ] )
	// t-ranking = OWS ";" OWS "q=" rank
	for i := from; i < edge; i++ {
		pair := &pairs[i]
		if pair.kind != pairHeader {
			continue
		}
		data := pair.dataAt(r.input)
		bytesToLower(data)
		if bytes.Equal(data, bytesTrailers) {
			r.acceptTrailers = true
		} else if r.httpVersion > Version1_1 {
			r.headResult, r.failReason = StatusBadRequest, "te codings other than trailers are not allowed in http/2 and http/3"
			return false
		}
	}
	return true
}
func (r *serverRequest_) checkUpgrade(pairs []pair, from uint8, edge uint8) bool { // Upgrade = #protocol
	if r.httpVersion > Version1_1 {
		r.headResult, r.failReason = StatusBadRequest, "upgrade is only supported in http/1"
		return false
	}
	if r.methodCode == MethodCONNECT {
		// TODO: confirm this
		return true
	}
	if r.httpVersion == Version1_1 {
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
			if b := r.input[fore]; httpHchar[b] == 1 {
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
	cookie.kind = pairCookie
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
func (r *serverRequest_) HasRanges() bool      { return r.nRanges > 0 }
func (r *serverRequest_) HasIfRange() bool     { return r.indexes.ifRange != 0 }
func (r *serverRequest_) UserAgent() string    { return string(r.UnsafeUserAgent()) }
func (r *serverRequest_) UnsafeUserAgent() []byte {
	if r.indexes.userAgent == 0 {
		return nil
	}
	return r.primes[r.indexes.userAgent].valueAt(r.input)
}

func (r *serverRequest_) addCookie(cookie *pair) bool { // as prime
	if edge, ok := r._addPrime(cookie); ok {
		r.cookies.edge = edge
		return true
	}
	r.headResult = StatusRequestHeaderFieldsTooLarge
	return false
}
func (r *serverRequest_) HasCookies() bool { return r.hasPairs(r.cookies, pairCookie) }
func (r *serverRequest_) AllCookies() (cookies [][2]string) {
	return r.allPairs(r.cookies, pairCookie)
}
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
	v, ok := r.getPair(name, 0, r.cookies, pairCookie)
	return string(v), ok
}
func (r *serverRequest_) UnsafeCookie(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.cookies, pairCookie)
}
func (r *serverRequest_) Cookies(name string) (values []string, ok bool) {
	return r.getPairs(name, 0, r.cookies, pairCookie)
}
func (r *serverRequest_) HasCookie(name string) bool {
	_, ok := r.getPair(name, 0, r.cookies, pairCookie)
	return ok
}
func (r *serverRequest_) DelCookie(name string) (deleted bool) {
	return r.delPair(name, 0, r.cookies, pairCookie)
}
func (r *serverRequest_) AddCookie(name string, value string) bool { // as extra
	return r.addExtra(name, value, 0, pairCookie)
}
func (r *serverRequest_) forCookies(callback func(cookie *pair, name []byte, value []byte) bool) bool {
	for i := r.cookies.from; i < r.cookies.edge; i++ {
		if cookie := &r.primes[i]; cookie.hash != 0 {
			if !callback(cookie, cookie.nameAt(r.input), cookie.valueAt(r.input)) {
				return false
			}
		}
	}
	if r.hasExtra[pairCookie] {
		for i := 0; i < len(r.extras); i++ {
			if extra := &r.extras[i]; extra.hash != 0 && extra.kind == pairCookie {
				if !callback(extra, extra.nameAt(r.array), extra.valueAt(r.array)) {
					return false
				}
			}
		}
	}
	return true
}

func (r *serverRequest_) EvalPreconditions(date int64, etag []byte, asOrigin bool) (status int16, normal bool) { // to test against preconditons intentionally
	// Get effective etag without ""
	if n := len(etag); n >= 2 && etag[0] == '"' && etag[n-1] == '"' {
		etag = etag[1 : n-1]
	}
	// See RFC 9110 (section 13.2.2).
	if asOrigin { // proxies may ignore if-match and if-unmodified-since.
		if r.ifMatch != 0 { // if-match is present
			if !r._evalIfMatch(etag) {
				return StatusPreconditionFailed, false
			}
		} else if r.indexes.ifUnmodifiedSince != 0 { // if-match is not present and if-unmodified-since is present
			if !r._evalIfUnmodifiedSince(date) {
				return StatusPreconditionFailed, false
			}
		}
	}
	getOrHead := r.methodCode&(MethodGET|MethodHEAD) != 0
	if r.ifNoneMatch != 0 { // if-none-match is present
		if !r._evalIfNoneMatch(etag) {
			if getOrHead {
				return StatusNotModified, false
			} else {
				return StatusPreconditionFailed, false
			}
		}
	} else if getOrHead && r.indexes.ifModifiedSince != 0 { // if-none-match is not present and if-modified-since is present
		if !r._evalIfModifiedSince(date) {
			return StatusNotModified, false
		}
	}
	return StatusOK, true
}
func (r *serverRequest_) _evalIfMatch(etag []byte) (pass bool) {
	if r.ifMatch == -1 { // *
		// If the field value is "*", the condition is true if the origin server has a current representation for the target resource.
		return true
	}
	for i := r.zones.ifMatch.from; i < r.zones.ifMatch.edge; i++ {
		header := &r.primes[i]
		if header.hash != hashIfMatch || !header.nameEqualBytes(r.input, bytesIfMatch) {
			continue
		}
		data := header.dataAt(r.input)
		if size := len(data); !(size >= 4 && data[0] == 'W' && data[1] == '/' && data[2] == '"' && data[size-1] == '"') && bytes.Equal(data, etag) {
			// If the field value is a list of entity tags, the condition is true if any of the listed tags match the entity tag of the selected representation.
			return true
		}
	}
	// TODO: r.extras?
	return false
}
func (r *serverRequest_) _evalIfNoneMatch(etag []byte) (pass bool) {
	if r.ifNoneMatch == -1 { // *
		// If the field value is "*", the condition is false if the origin server has a current representation for the target resource.
		return false
	}
	for i := r.zones.ifNoneMatch.from; i < r.zones.ifNoneMatch.edge; i++ {
		header := &r.primes[i]
		if header.hash != hashIfNoneMatch || !header.nameEqualBytes(r.input, bytesIfNoneMatch) {
			continue
		}
		if bytes.Equal(header.valueAt(r.input), etag) {
			// If the field value is a list of entity tags, the condition is false if one of the listed tags matches the entity tag of the selected representation.
			return false
		}
	}
	// TODO: r.extras?
	return true
}
func (r *serverRequest_) _evalIfModifiedSince(date int64) (pass bool) {
	// If the selected representation's last modification date is earlier than or equal to the date provided in the field value, the condition is false.
	return date > r.unixTimes.ifModifiedSince
}
func (r *serverRequest_) _evalIfUnmodifiedSince(date int64) (pass bool) {
	// If the selected representation's last modification date is earlier than or equal to the date provided in the field value, the condition is true.
	return date <= r.unixTimes.ifUnmodifiedSince
}

func (r *serverRequest_) EvalIfRange(date int64, etag []byte, asOrigin bool) (canRange bool) {
	if r.unixTimes.ifRange == 0 {
		if r._evalIfRangeETag(etag) {
			return true
		}
	} else if r._evalIfRangeDate(date) {
		return true
	}
	return false
}
func (r *serverRequest_) _evalIfRangeETag(etag []byte) (pass bool) {
	ifRange := &r.primes[r.indexes.ifRange] // TODO: r.extras?
	data := ifRange.dataAt(r.input)
	if size := len(data); !(size >= 4 && data[0] == 'W' && data[1] == '/' && data[2] == '"' && data[size-1] == '"') && bytes.Equal(data, etag) {
		// If the entity-tag validator provided exactly matches the ETag field value for the selected representation using the strong comparison function (Section 8.8.3.2), the condition is true.
		return true
	}
	return false
}
func (r *serverRequest_) _evalIfRangeDate(date int64) (pass bool) {
	// If the HTTP-date validator provided exactly matches the Last-Modified field value for the selected representation, the condition is true.
	return r.unixTimes.ifRange == date
}

func (r *serverRequest_) EvalRanges(contentSize int64) []Range { // returned ranges are converted from [from:last] to the format of [from:edge)
	rangedSize := int64(0)
	for i := int8(0); i < r.nRanges; i++ {
		rang := &r.ranges[i]
		if rang.From == -1 { // "-" suffix-length, means the last `suffix-length` bytes
			if rang.Last == 0 {
				return nil
			}
			if rang.Last >= contentSize {
				rang.From = 0
			} else {
				rang.From = contentSize - rang.Last
			}
			rang.Last = contentSize
		} else { // first-pos "-" [ last-pos ]
			if rang.From >= contentSize {
				return nil
			}
			if rang.Last == -1 { // first-pos "-", to the end if last-pos is not present
				rang.Last = contentSize
			} else { // first-pos "-" last-pos
				if rang.Last >= contentSize {
					rang.Last = contentSize
				} else {
					rang.Last++
				}
			}
		}
		rangedSize += rang.Last - rang.From
		if rangedSize > contentSize { // possible attack
			return nil
		}
	}
	return r.ranges[:r.nRanges]
}

func (r *serverRequest_) unsetHost() { // used by proxies
	r._delPrime(r.indexes.host) // zero safe
}

func (r *serverRequest_) HasContent() bool { return r.contentSize >= 0 || r.IsVague() }
func (r *serverRequest_) Content() string  { return string(r.UnsafeContent()) }
func (r *serverRequest_) UnsafeContent() []byte {
	if r.formKind == httpFormMultipart { // loading multipart form into memory is not allowed!
		return nil
	}
	return r.unsafeContent()
}

func (r *serverRequest_) parseHTMLForm() { // to populate r.forms and r.upfiles
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
	r._loadContent()
	if r.stream.isBroken() {
		return
	}
	var (
		state = 2 // to be consistent with r._recvControl() in HTTP/1
		octet byte
	)
	form := &r.mainPair
	form.zero()
	form.kind = pairForm
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
			nybble, ok := byteFromHex(b)
			if !ok {
				r.bodyResult, r.failReason = StatusBadRequest, "invalid pct encoding"
				return
			}
			if state&0xf == 0xf { // expecting the first HEXDIG
				octet = nybble << 4
				state &= 0xf0 // this reserves last state and leads to the state of second HEXDIG
			} else { // expecting the second HEXDIG
				octet |= nybble
				if state == 0x20 { // in name, calculate name hash
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
		// r.contentText is set, r.contentTextKind == httpContentTextInput. r.formWindow refers to the exact r.contentText.
		r.formWindow = r.contentText
		r.formEdge = int32(len(r.formWindow))
	} else { // content is not received
		r.contentReceived = true
		switch content := r._recvContent(true).(type) { // retain
		case []byte: // (0, 64K1]. case happens when sized content <= 64K1
			r.contentText = content
			r.contentTextKind = httpContentTextPool        // so r.contentText can be freed on end
			r.formWindow = r.contentText[0:r.receivedSize] // r.formWindow refers to the exact r.content.
			r.formEdge = int32(r.receivedSize)
		case tempFile: // [0, r.webapp.maxUpfileSize]. case happens when sized content > 64K1, or content is vague.
			r.contentFile = content.(*os.File)
			if r.receivedSize == 0 {
				return // vague content can be empty
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
			if DebugLevel() >= 2 {
				Println(r.arrayEdge, cap(r.array), string(r.array[0:r.arrayEdge]))
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
			upfile Upfile   // if part is a file, this is used. zeroed
		}
		part.form.kind = pairForm
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
						if m := para.value.size(); m == 0 || m > 255 {
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
						// TODO: Is this a good implementation? If size is too large, just use bytes.Equal? Use a special hash value (like 0xffff) to hint this?
						for p := para.value.from; p < para.value.edge; p++ {
							part.hash += uint16(r.formWindow[p])
						}
					} else if bytes.Equal(paraName, bytesFilename) { // filename="michael.jpg"
						if m := para.value.size(); m == 0 || m > 255 {
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
						if !r.arrayCopy(ConstBytes(r.saveContentFilesDir())) { // add "/path/to/"
							r.stream.markBroken()
							return
						}
						nameBuffer := r.stream.buffer256() // enough for tempName
						m := r.stream.httpConn().MakeTempName(nameBuffer, r.recvTime.Unix())
						if !r.arrayCopy(nameBuffer[:m]) { // add "391384576"
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
			part.upfile.hash = part.hash
			part.upfile.nameSize, part.upfile.nameFrom = uint8(part.name.size()), part.name.from
			part.upfile.baseSize, part.upfile.baseFrom = uint8(part.base.size()), part.base.from
			part.upfile.typeSize, part.upfile.typeFrom = uint8(part.type_.size()), part.type_.from
			part.upfile.pathSize, part.upfile.pathFrom = uint8(part.path.size()), part.path.from
			if osFile, err := os.OpenFile(WeakString(r.array[part.path.from:part.path.edge]), os.O_RDWR|os.O_CREATE, 0644); err == nil {
				if DebugLevel() >= 2 {
					Println("OPENED")
				}
				part.osFile = osFile
			} else {
				if DebugLevel() >= 2 {
					Println(err.Error())
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
					r.addUpfile(&part.upfile)
					part.osFile.Close()
					if DebugLevel() >= 2 {
						Println("CLOSED")
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
func (r *serverRequest_) HasForms() bool {
	r.parseHTMLForm()
	return r.hasPairs(r.forms, pairForm)
}
func (r *serverRequest_) AllForms() (forms [][2]string) {
	r.parseHTMLForm()
	return r.allPairs(r.forms, pairForm)
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
	v, ok := r.getPair(name, 0, r.forms, pairForm)
	return string(v), ok
}
func (r *serverRequest_) UnsafeForm(name string) (value []byte, ok bool) {
	r.parseHTMLForm()
	return r.getPair(name, 0, r.forms, pairForm)
}
func (r *serverRequest_) Forms(name string) (values []string, ok bool) {
	r.parseHTMLForm()
	return r.getPairs(name, 0, r.forms, pairForm)
}
func (r *serverRequest_) HasForm(name string) bool {
	r.parseHTMLForm()
	_, ok := r.getPair(name, 0, r.forms, pairForm)
	return ok
}
func (r *serverRequest_) DelForm(name string) (deleted bool) {
	r.parseHTMLForm()
	return r.delPair(name, 0, r.forms, pairForm)
}
func (r *serverRequest_) AddForm(name string, value string) bool { // as extra
	return r.addExtra(name, value, 0, pairForm)
}

func (r *serverRequest_) addUpfile(upfile *Upfile) {
	if len(r.upfiles) == cap(r.upfiles) {
		if cap(r.upfiles) == cap(r.stockUpfiles) {
			upfiles := make([]Upfile, 0, 16)
			r.upfiles = append(upfiles, r.upfiles...)
		} else if cap(r.upfiles) == 16 {
			upfiles := make([]Upfile, 0, 128)
			r.upfiles = append(upfiles, r.upfiles...)
		} else {
			// Ignore too many upfiles
			return
		}
	}
	r.upfiles = append(r.upfiles, *upfile)
}
func (r *serverRequest_) HasUpfiles() bool {
	r.parseHTMLForm()
	return len(r.upfiles) != 0
}
func (r *serverRequest_) AllUpfiles() (upfiles []*Upfile) {
	r.parseHTMLForm()
	for i := 0; i < len(r.upfiles); i++ {
		upfile := &r.upfiles[i]
		upfile.setMeta(r.array)
		upfiles = append(upfiles, upfile)
	}
	return upfiles
}
func (r *serverRequest_) U(name string) *Upfile {
	upfile, _ := r.Upfile(name)
	return upfile
}
func (r *serverRequest_) Upfile(name string) (upfile *Upfile, ok bool) {
	r.parseHTMLForm()
	if n := len(r.upfiles); n > 0 && name != "" {
		hash := stringHash(name)
		for i := 0; i < n; i++ {
			if upfile := &r.upfiles[i]; upfile.hash == hash && upfile.nameEqualString(r.array, name) {
				upfile.setMeta(r.array)
				return upfile, true
			}
		}
	}
	return
}
func (r *serverRequest_) Upfiles(name string) (upfiles []*Upfile, ok bool) {
	r.parseHTMLForm()
	if n := len(r.upfiles); n > 0 && name != "" {
		hash := stringHash(name)
		for i := 0; i < n; i++ {
			if upfile := &r.upfiles[i]; upfile.hash == hash && upfile.nameEqualString(r.array, name) {
				upfile.setMeta(r.array)
				upfiles = append(upfiles, upfile)
			}
		}
		if len(upfiles) > 0 {
			ok = true
		}
	}
	return
}
func (r *serverRequest_) HasUpfile(name string) bool {
	r.parseHTMLForm()
	_, ok := r.Upfile(name)
	return ok
}

func (r *serverRequest_) examineTail() bool {
	for i := r.trailers.from; i < r.trailers.edge; i++ {
		if !r.applyTrailer(i) {
			// r.bodyResult is set.
			return false
		}
	}
	return true
}
func (r *serverRequest_) applyTrailer(index uint8) bool {
	//trailer := &r.primes[index]
	// TODO: Pseudo-header fields MUST NOT appear in a trailer section.
	return true
}

func (r *serverRequest_) hookReviser(reviser Reviser) {
	r.hasRevisers = true
	r.revisers[reviser.Rank()] = reviser.ID() // revisers are placed to fixed position, by their ranks.
}

func (r *serverRequest_) unsafeVariable(code int16, name string) (value []byte) {
	if code != -1 {
		return serverRequestVariables[code](r)
	}
	if strings.HasPrefix(name, "header_") {
		name = name[len("header_"):]
		if v, ok := r.UnsafeHeader(name); ok {
			return v
		}
	} else if strings.HasPrefix(name, "query_") {
		name = name[len("query_"):]
		if v, ok := r.UnsafeQuery(name); ok {
			return v
		}
	} else if strings.HasPrefix(name, "cookie_") {
		name = name[len("cookie_"):]
		if v, ok := r.UnsafeCookie(name); ok {
			return v
		}
	}
	return nil
}

var serverRequestVariables = [...]func(*serverRequest_) []byte{ // keep sync with varCodes
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

// Upfile is a file uploaded by http client.
type Upfile struct { // 48 bytes
	hash     uint16 // hash of name, to support fast comparison
	flags    uint8  // see upfile flags
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

func (u *Upfile) nameEqualString(p []byte, x string) bool {
	if int(u.nameSize) != len(x) {
		return false
	}
	if u.metaSet() {
		return u.meta[u.nameFrom:u.nameFrom+int32(u.nameSize)] == x
	}
	return string(p[u.nameFrom:u.nameFrom+int32(u.nameSize)]) == x
}

const ( // upfile flags
	upfileFlagMetaSet = 0b10000000
	upfileFlagIsMoved = 0b01000000
)

func (u *Upfile) setMeta(p []byte) {
	if u.flags&upfileFlagMetaSet > 0 {
		return
	}
	u.flags |= upfileFlagMetaSet
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
func (u *Upfile) metaSet() bool { return u.flags&upfileFlagMetaSet > 0 }
func (u *Upfile) setMoved()     { u.flags |= upfileFlagIsMoved }
func (u *Upfile) isMoved() bool { return u.flags&upfileFlagIsMoved > 0 }

const ( // upfile error codes
	upfileOK        = 0
	upfileError     = 1
	upfileCantWrite = 2
	upfileTooLarge  = 3
	upfilePartial   = 4
	upfileNoFile    = 5
)

var upfileErrors = [...]error{
	nil, // no error
	errors.New("general error"),
	errors.New("cannot write"),
	errors.New("too large"),
	errors.New("partial"),
	errors.New("no file"),
}

func (u *Upfile) IsOK() bool   { return u.errCode == 0 }
func (u *Upfile) Error() error { return upfileErrors[u.errCode] }

func (u *Upfile) Name() string { return u.meta[u.nameFrom : u.nameFrom+int32(u.nameSize)] }
func (u *Upfile) Base() string { return u.meta[u.baseFrom : u.baseFrom+int32(u.baseSize)] }
func (u *Upfile) Type() string { return u.meta[u.typeFrom : u.typeFrom+int32(u.typeSize)] }
func (u *Upfile) Path() string { return u.meta[u.pathFrom : u.pathFrom+int32(u.pathSize)] }
func (u *Upfile) Size() int64  { return u.size }

func (u *Upfile) MoveTo(path string) error {
	// TODO. Remember to mark as moved
	return nil
}

// Response is the server-side http response.
type Response interface { // for *server[1-3]Response
	Request() Request

	SetStatus(status int16) error
	Status() int16

	MakeETagFrom(date int64, size int64) ([]byte, bool) // with `""`
	SetExpires(expires int64) bool
	SetLastModified(lastModified int64) bool
	AddContentType(contentType string) bool
	AddContentTypeBytes(contentType []byte) bool
	AddHTTPSRedirection(authority string) bool
	AddHostnameRedirection(hostname string) bool
	AddDirectoryRedirection() bool

	AddCookie(cookie *Cookie) bool

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
	SendFile(contentPath string) error
	SendJSON(content any) error
	SendBadRequest(content []byte) error                             // 400
	SendForbidden(content []byte) error                              // 403
	SendNotFound(content []byte) error                               // 404
	SendMethodNotAllowed(allow string, content []byte) error         // 405
	SendRangeNotSatisfiable(contentSize int64, content []byte) error // 416
	SendInternalServerError(content []byte) error                    // 500
	SendNotImplemented(content []byte) error                         // 501
	SendBadGateway(content []byte) error                             // 502
	SendGatewayTimeout(content []byte) error                         // 504

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
	pickRanges(ranges []Range, rangeType string)
	sendText(content []byte) error
	sendFile(content *os.File, info os.FileInfo, shut bool) error // will close content after sent
	sendChain() error                                             // content
	echoHeaders() error
	echoChain() error // chunks
	addTrailer(name []byte, value []byte) bool
	endVague() error
	proxyPass1xx(resp backendResponse) bool
	proxyPass(resp backendResponse) error
	proxyPost(content any, hasTrailers bool) error
	proxyCopyHead(resp backendResponse, cfg *WebExchanProxyConfig) bool
	proxyCopyTail(resp backendResponse, cfg *WebExchanProxyConfig) bool
	hookReviser(reviser Reviser)
	unsafeMake(size int) []byte
}

// serverResponse_ is the parent for server[1-3]Response.
type serverResponse_ struct { // outgoing. needs building
	// Parent
	httpOut_ // outgoing http message
	// Assocs
	request Request // related request
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	status    int16    // 200, 302, 404, 500, ...
	_         [6]byte  // padding
	start     [16]byte // exactly 16 bytes for "HTTP/1.1 xxx ?\r\n". also used by HTTP/2 and HTTP/3, but shorter
	unixTimes struct { // in seconds
		expires      int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
		lastModified int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
	}
	// Stream states (zeros)
	webapp          *Webapp // associated webapp
	serverResponse0         // all values must be zero by default in this struct!
}
type serverResponse0 struct { // for fast reset, entirely
	indexes struct {
		expires      uint8
		lastModified uint8
		_            [6]byte // padding
	}
	revisers [32]uint8 // reviser ids which will apply on this response. indexed by reviser order
}

func (r *serverResponse_) onUse(httpVersion uint8) { // for non-zeros
	const asRequest = false
	r.httpOut_.onUse(httpVersion, asRequest)
	r.status = StatusOK
	r.unixTimes.expires = -1      // not set
	r.unixTimes.lastModified = -1 // not set
}
func (r *serverResponse_) onEnd() { // for zeros
	r.webapp = nil
	r.serverResponse0 = serverResponse0{}
	r.httpOut_.onEnd()
}

func (r *serverResponse_) Request() Request { return r.request }

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
		return httpOutUnknownStatus
	}
}
func (r *serverResponse_) Status() int16 { return r.status }

func (r *serverResponse_) MakeETagFrom(date int64, size int64) ([]byte, bool) { // with ""
	if date < 0 || size < 0 {
		return nil, false
	}
	p := r.unsafeMake(32)
	p[0] = '"'
	etag := p[1:]
	n := i64ToHex(date, etag)
	etag[n] = '-'
	if n++; n > 13 {
		return nil, false
	}
	n = 1 + n + i64ToHex(size, etag[n:])
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
	r.AddHeaderBytes(bytesAllow, ConstBytes(allow))
	return r.sendError(StatusMethodNotAllowed, content)
}
func (r *serverResponse_) SendRangeNotSatisfiable(contentSize int64, content []byte) error { // 416
	// add a header like: content-range: bytes */1234
	valueBuffer := r.stream.buffer256()
	n := copy(valueBuffer, bytesBytesStarSlash)
	n += i64ToDec(contentSize, valueBuffer[n:])
	r.AddHeaderBytes(bytesContentRange, valueBuffer[:n])
	return r.sendError(StatusRangeNotSatisfiable, content)
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
		content = serverErrorPages[status]
	}
	r.piece.SetText(content)
	r.chain.PushTail(&r.piece)
	r.contentSize = int64(len(content))
	return r.shell.sendChain()
}

var serverErrorPages = func() map[int16][]byte {
	const template = `<!doctype html>
<html lang="en">
<head>
<meta name="viewport" content="width=device-width,initial-scale=1.0">
<meta charset="utf-8">
<title>%d %s</title>
<style type="text/css">
body{text-align:center;}
header{font-size:72pt;}
main{font-size:36pt;}
footer{padding:20px;}
</style>
</head>
<body>
	<header>%d</header>
	<main>%s</main>
	<footer>Powered by Gorox</footer>
</body>
</html>`
	pages := make(map[int16][]byte)
	for status, control := range http1Controls {
		if status < 400 || control == nil {
			continue
		}
		phrase := control[len("HTTP/1.1 XXX ") : len(control)-2]
		pages[int16(status)] = []byte(fmt.Sprintf(template, status, phrase, status, phrase))
	}
	return pages
}()

func (r *serverResponse_) beforeSend() {
	resp := r.shell.(Response)
	for _, id := range r.revisers { // revise headers
		if id == 0 { // id of effective reviser is ensured to be > 0
			continue
		}
		reviser := r.webapp.reviserByID(id)
		reviser.BeforeSend(resp.Request(), resp)
	}
}
func (r *serverResponse_) doSend() error {
	if r.hasRevisers {
		resp := r.shell.(Response)
		for _, id := range r.revisers { // revise sized content
			if id == 0 {
				continue
			}
			reviser := r.webapp.reviserByID(id)
			reviser.OnOutput(resp.Request(), resp, &r.chain)
		}
		// Because r.chain may be altered by revisers, content size must be recalculated
		if contentSize, ok := r.chain.Size(); ok {
			r.contentSize = contentSize
		} else {
			return httpOutTooLarge
		}
	}
	return r.shell.sendChain()
}

func (r *serverResponse_) beforeEcho() {
	resp := r.shell.(Response)
	for _, id := range r.revisers { // revise headers
		if id == 0 { // id of effective reviser is ensured to be > 0
			continue
		}
		reviser := r.webapp.reviserByID(id)
		reviser.BeforeEcho(resp.Request(), resp)
	}
}
func (r *serverResponse_) doEcho() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	r.chain.PushTail(&r.piece)
	defer r.chain.free()
	if r.hasRevisers {
		resp := r.shell.(Response)
		for _, id := range r.revisers { // revise vague content
			if id == 0 { // id of effective reviser is ensured to be > 0
				continue
			}
			reviser := r.webapp.reviserByID(id)
			reviser.OnOutput(resp.Request(), resp, &r.chain)
		}
	}
	return r.shell.echoChain()
}
func (r *serverResponse_) endVague() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	if r.hasRevisers {
		resp := r.shell.(Response)
		for _, id := range r.revisers { // finish vague content
			if id == 0 { // id of effective reviser is ensured to be > 0
				continue
			}
			reviser := r.webapp.reviserByID(id)
			reviser.FinishEcho(resp.Request(), resp)
		}
	}
	return r.shell.finalizeVague()
}

var ( // perfect hash table for response critical headers
	serverResponseCriticalHeaderTable = [10]struct {
		hash uint16
		name []byte
		fAdd func(*serverResponse_, []byte) (ok bool)
		fDel func(*serverResponse_) (deleted bool)
	}{ // connection content-length content-type date expires last-modified server set-cookie transfer-encoding upgrade
		0: {hashServer, bytesServer, nil, nil},       // restricted
		1: {hashSetCookie, bytesSetCookie, nil, nil}, // restricted
		2: {hashUpgrade, bytesUpgrade, nil, nil},     // restricted
		3: {hashDate, bytesDate, (*serverResponse_).appendDate, (*serverResponse_).deleteDate},
		4: {hashTransferEncoding, bytesTransferEncoding, nil, nil}, // restricted
		5: {hashConnection, bytesConnection, nil, nil},             // restricted
		6: {hashLastModified, bytesLastModified, (*serverResponse_).appendLastModified, (*serverResponse_).deleteLastModified},
		7: {hashExpires, bytesExpires, (*serverResponse_).appendExpires, (*serverResponse_).deleteExpires},
		8: {hashContentLength, bytesContentLength, nil, nil}, // restricted
		9: {hashContentType, bytesContentType, (*serverResponse_).appendContentType, (*serverResponse_).deleteContentType},
	}
	serverResponseCriticalHeaderFind = func(hash uint16) int { return (113100 / int(hash)) % 10 }
)

func (r *serverResponse_) insertHeader(hash uint16, name []byte, value []byte) bool {
	h := &serverResponseCriticalHeaderTable[serverResponseCriticalHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(h.name, name) {
		if h.fAdd == nil { // mainly because this header is restricted to insert
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
		if h.fDel == nil { // mainly because this header is restricted to remove
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

func (r *serverResponse_) proxyPass(resp backendResponse) error { // sync content to the other side directly
	pass := r.shell.passBytes
	if resp.IsVague() || r.hasRevisers { // if we need to revise, we always use vague no matter the original content is sized or vague
		pass = r.EchoBytes
	} else { // resp is sized and there are no revisers, use passBytes
		r.isSent = true
		r.contentSize = resp.ContentSize()
		// TODO: find a way to reduce i/o syscalls if content is small?
		if err := r.shell.passHeaders(); err != nil {
			return err
		}
	}
	for {
		p, err := resp.readContent()
		if len(p) >= 0 {
			if e := pass(p); e != nil {
				return e
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	if resp.HasTrailers() { // added trailers will be written by upper code eventually.
		if !resp.forTrailers(func(trailer *pair, name []byte, value []byte) bool {
			return r.shell.addTrailer(name, value)
		}) {
			return httpOutTrailerFailed
		}
	}
	return nil
}
func (r *serverResponse_) proxyCopyHead(resp backendResponse, cfg *WebExchanProxyConfig) bool {
	resp.delHopHeaders()

	// copy control (:status)
	r.SetStatus(resp.Status())

	// copy selective forbidden headers (excluding set-cookie, which is copied directly) from resp

	// copy remaining headers from resp
	if !resp.forHeaders(func(header *pair, name []byte, value []byte) bool {
		if header.hash == hashSetCookie && bytes.Equal(name, bytesSetCookie) { // set-cookie is copied directly
			return r.shell.addHeader(name, value)
		} else {
			return r.shell.insertHeader(header.hash, name, value)
		}
	}) {
		return false
	}

	return true
}
func (r *serverResponse_) proxyCopyTail(resp backendResponse, cfg *WebExchanProxyConfig) bool {
	return resp.forTrailers(func(trailer *pair, name []byte, value []byte) bool {
		return r.shell.addTrailer(name, value)
	})
}

func (r *serverResponse_) hookReviser(reviser Reviser) {
	r.hasRevisers = true
	r.revisers[reviser.Rank()] = reviser.ID() // revisers are placed to fixed position, by their ranks.
}

// Cookie is a "set-cookie" header sent to client.
type Cookie struct {
	name     string
	value    string
	expires  time.Time
	domain   string
	path     string
	sameSite string
	maxAge   int32
	secure   bool
	httpOnly bool
	invalid  bool
	quote    bool // if true, quote value with ""
	aSize    int8
	ageBuf   [10]byte
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
func (c *Cookie) SetMaxAge(maxAge int32)  { c.maxAge = maxAge }
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
		m := i32ToDec(c.maxAge, c.ageBuf[:])
		c.aSize = int8(m)
		n += len("; Max-Age=") + m
	} else if c.maxAge < 0 {
		c.ageBuf[0] = '0'
		c.aSize = 1
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
		i += copy(p[i:], c.ageBuf[0:c.aSize])
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

// Socket is the server-side websocket.
type Socket interface { // for *server[1-3]Socket
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
}

// serverSocket_ is the parent for server[1-3]Socket.
type serverSocket_ struct {
	// Parent
	webSocket_
	// Assocs
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *serverSocket_) onUse() {
	s.webSocket_.onUse()
}
func (s *serverSocket_) onEnd() {
	s.webSocket_.onEnd()
}

// Webapp is the Web application.
type Webapp struct {
	// Parent
	Component_
	// Assocs
	stage    *Stage            // current stage
	stater   Stater            // the stater which is used by this webapp
	servers  []HTTPServer      // bound http servers. may be empty
	handlets compDict[Handlet] // defined handlets. indexed by name
	revisers compDict[Reviser] // defined revisers. indexed by name
	socklets compDict[Socklet] // defined socklets. indexed by name
	rules    compList[*Rule]   // defined rules. the order must be kept, so we use list. TODO: use ordered map?
	// States
	hostnames       [][]byte          // like: ("www.example.com", "1.2.3.4", "fff8::1")
	webRoot         string            // root dir for the web
	file404         string            // 404 file path
	text404         []byte            // bytes of the default 404 file
	tlsCertificate  string            // tls certificate file, in pem format
	tlsPrivateKey   string            // tls private key file, in pem format
	accessLog       *LogConfig        // ...
	logger          *Logger           // webapp access logger
	maxUpfileSize   int64             // max content size that uploads files through multipart/form-data
	settings        map[string]string // webapp settings defined and used by users
	settingsLock    sync.RWMutex      // protects settings
	isDefault       bool              // is this webapp the default webapp of its belonging http servers?
	exactHostnames  [][]byte          // like: ("example.com")
	suffixHostnames [][]byte          // like: ("*.example.com")
	prefixHostnames [][]byte          // like: ("www.example.*")
	revisersByID    [256]Reviser      // for fast searching. position 0 is not used
	nRevisers       uint8             // used number of revisersByID in this webapp
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
	close(a.ShutChan) // notifies maintain() which shutdown sub components
}

func (a *Webapp) OnConfigure() {
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

	// maxUpfileSize
	a.ConfigureInt64("maxUpfileSize", &a.maxUpfileSize, func(value int64) error {
		if value > 0 && value <= _1T {
			return nil
		}
		return errors.New(".maxUpfileSize has an invalid value")
	}, _128M)

	// settings
	a.ConfigureStringDict("settings", &a.settings, nil, make(map[string]string))

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
	if a.accessLog != nil {
		//a.logger = NewLogger(a.accessLog.logFile, a.accessLog.rotate)
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

func (a *Webapp) maintain() { // runner
	a.Loop(time.Second, func(now time.Time) {
		// TODO
	})

	a.IncSubs(len(a.handlets) + len(a.revisers) + len(a.socklets) + len(a.rules))
	a.rules.goWalk((*Rule).OnShutdown)
	a.socklets.goWalk(Socklet.OnShutdown)
	a.revisers.goWalk(Reviser.OnShutdown)
	a.handlets.goWalk(Handlet.OnShutdown)
	a.WaitSubs() // handlets, revisers, socklets, rules

	if a.logger != nil {
		a.logger.Close()
	}
	if DebugLevel() >= 2 {
		Printf("webapp=%s done\n", a.Name())
	}
	a.stage.DecSub() // webapp
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

func (a *Webapp) bindServer(server HTTPServer) { a.servers = append(a.servers, server) }

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

func (a *Webapp) dispatchExchan(req Request, resp Response) {
	req.makeAbsPath() // for fs check rules, if any
	for _, rule := range a.rules {
		if !rule.isMatch(req) {
			continue
		}
		if handled := rule.executeExchan(req, resp); handled {
			if rule.logAccess && a.logger != nil {
				//a.logger.logf("status=%d %s %s\n", resp.Status(), req.Method(), req.UnsafeURI())
			}
			return
		}
	}
	// If we reach here, it means the exchan is not handled by any rules or handlets in this webapp.
	resp.SendNotFound(a.text404)
}
func (a *Webapp) dispatchSocket(req Request, sock Socket) {
	req.makeAbsPath() // for fs check rules, if any
	for _, rule := range a.rules {
		if !rule.isMatch(req) {
			continue
		}
		if served := rule.executeSocket(req, sock); served {
			if rule.logAccess && a.logger != nil {
				// TODO: log?
			}
			return
		}
	}
	// If we reach here, it means the socket is not served by any rules or socklets in this webapp.
	sock.Close()
}

// Rule component
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
	matcher    func(rule *Rule, req Request, value []byte) bool
}

func (r *Rule) onCreate(name string, webapp *Webapp) {
	r.MakeComp(name)
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

func (r *Rule) executeExchan(req Request, resp Response) (handled bool) {
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
					resp.(*server1Response).setConnectionClose() // A proxy server MUST NOT maintain a persistent connection with an HTTP/1.0 client.
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
func (r *Rule) executeSocket(req Request, sock Socket) (served bool) {
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

func (r *Rule) equalMatch(req Request, value []byte) bool { // value == patterns
	return equalMatch(value, r.patterns)
}
func (r *Rule) prefixMatch(req Request, value []byte) bool { // value ^= patterns
	return prefixMatch(value, r.patterns)
}
func (r *Rule) suffixMatch(req Request, value []byte) bool { // value $= patterns
	return suffixMatch(value, r.patterns)
}
func (r *Rule) containMatch(req Request, value []byte) bool { // value *= patterns
	return containMatch(value, r.patterns)
}
func (r *Rule) regexpMatch(req Request, value []byte) bool { // value ~= patterns
	return regexpMatch(value, r.regexps)
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
	return notEqualMatch(value, r.patterns)
}
func (r *Rule) notPrefixMatch(req Request, value []byte) bool { // value !^ patterns
	return notPrefixMatch(value, r.patterns)
}
func (r *Rule) notSuffixMatch(req Request, value []byte) bool { // value !$ patterns
	return notSuffixMatch(value, r.patterns)
}
func (r *Rule) notContainMatch(req Request, value []byte) bool { // value !* patterns
	return notContainMatch(value, r.patterns)
}
func (r *Rule) notRegexpMatch(req Request, value []byte) bool { // value !~ patterns
	return notRegexpMatch(value, r.regexps)
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

// Handle is a function which handles http request and gives http response.
type Handle func(req Request, resp Response)

// Mapper performs request mapping in handlets. Mappers are not components.
type Mapper interface {
	FindHandle(req Request) Handle // called firstly
	HandleName(req Request) string // called secondly
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

// Handlet_ is the parent for all handlets.
type Handlet_ struct {
	// Parent
	Component_
	// Assocs
	mapper Mapper
	// States
	rShell reflect.Value // the shell handlet
}

func (h *Handlet_) IsProxy() bool { return false } // override this for proxy handlets
func (h *Handlet_) IsCache() bool { return false } // override this for cache handlets

func (h *Handlet_) UseMapper(handlet Handlet, mapper Mapper) {
	h.mapper = mapper
	h.rShell = reflect.ValueOf(handlet)
}
func (h *Handlet_) Dispatch(req Request, resp Response, notFound Handle) {
	if h.mapper != nil {
		if handle := h.mapper.FindHandle(req); handle != nil {
			handle(req, resp)
			return
		}
		if name := h.mapper.HandleName(req); name != "" {
			if rMethod := h.rShell.MethodByName(name); rMethod.IsValid() {
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

// Reviser component revises incoming requests and outgoing responses.
type Reviser interface {
	// Imports
	Component
	// Methods
	ID() uint8
	setID(id uint8)
	Rank() int8 // 0-31 (with 0-15 as tunable, 16-31 as fixed)

	BeforeRecv(req Request, resp Response) // for sized content
	BeforeDraw(req Request, resp Response) // for vague content
	OnInput(req Request, resp Response, chain *Chain) bool
	FinishDraw(req Request, resp Response) // for vague content

	BeforeSend(req Request, resp Response) // for sized content
	BeforeEcho(req Request, resp Response) // for vague content
	OnOutput(req Request, resp Response, chain *Chain)
	FinishEcho(req Request, resp Response) // for vague content
}

// Reviser_ is the parent for all revisers.
type Reviser_ struct {
	// Parent
	Component_
	// States
	id uint8
}

func (r *Reviser_) ID() uint8      { return r.id }
func (r *Reviser_) setID(id uint8) { r.id = id }

// Socklet component handles the websocket.
type Socklet interface {
	// Imports
	Component
	// Methods
	IsProxy() bool // proxys and origins are different, we must differentiate them
	Serve(req Request, sock Socket)
}

// Socklet_ is the parent for all socklets.
type Socklet_ struct {
	// Parent
	Component_
	// States
}

func (s *Socklet_) IsProxy() bool { return false } // override this for proxy socklets

// Stater component is the interface to storages of Web states.
type Stater interface {
	// Imports
	Component
	// Methods
	Maintain() // runner
	Set(sid []byte, session *Session)
	Get(sid []byte) (session *Session)
	Del(sid []byte) bool
}

// Stater_ is the parent for all staters.
type Stater_ struct {
	// Parent
	Component_
}

// Session is a Web session in stater
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

// staticHandlet is the classic http handlet for static files and directories.
type staticHandlet struct {
	// Parent
	Handlet_
	// Assocs
	stage  *Stage // current stage
	webapp *Webapp
	// States
	webRoot       string            // root dir for web files and directories
	aliasTo       []string          // from is an alias to to
	indexFile     string            // the file that will be used as index
	autoIndex     bool              // list files in directories if there is no index file?
	mimeTypes     map[string]string // defined mime types for file extensions
	defaultType   string            // mime type for file extensions that are not defined in mimeTypes
	useAppWebRoot bool              // true if webRoot is same with webapp.webRoot
}

func (h *staticHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *staticHandlet) OnShutdown() {
	h.webapp.DecSub() // handlet
}

func (h *staticHandlet) OnConfigure() {
	// webRoot
	if v, ok := h.Find("webRoot"); ok {
		if dir, ok := v.String(); ok && dir != "" {
			h.webRoot = dir
		} else {
			UseExitln("invalid webRoot")
		}
	} else {
		UseExitln("webRoot is required for staticHandlet")
	}
	h.webRoot = strings.TrimRight(h.webRoot, "/")
	h.useAppWebRoot = h.webRoot == h.webapp.webRoot
	if DebugLevel() >= 1 {
		if h.useAppWebRoot {
			Printf("static=%s use webapp web root\n", h.Name())
		} else {
			Printf("static=%s NOT use webapp web root\n", h.Name())
		}
	}

	// aliasTo
	if v, ok := h.Find("aliasTo"); ok {
		if fromTo, ok := v.StringListN(2); ok {
			h.aliasTo = fromTo
		} else {
			UseExitln("invalid aliasTo")
		}
	} else {
		h.aliasTo = nil
	}

	// indexFile
	h.ConfigureString("indexFile", &h.indexFile, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".indexFile has an invalid value")
	}, "index.html")

	// mimeTypes
	if v, ok := h.Find("mimeTypes"); ok {
		if mimeTypes, ok := v.StringDict(); ok {
			h.mimeTypes = make(map[string]string)
			for ext, mimeType := range staticDefaultMimeTypes {
				h.mimeTypes[ext] = mimeType
			}
			for ext, mimeType := range mimeTypes { // overwrite default
				h.mimeTypes[ext] = mimeType
			}
		} else {
			UseExitln("invalid mimeTypes")
		}
	} else {
		h.mimeTypes = staticDefaultMimeTypes
	}

	// defaultType
	h.ConfigureString("defaultType", &h.defaultType, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".indexFile has an invalid value")
	}, "application/octet-stream")

	// autoIndex
	h.ConfigureBool("autoIndex", &h.autoIndex, false)
}
func (h *staticHandlet) OnPrepare() {
	if info, err := os.Stat(h.webRoot + "/" + h.indexFile); err == nil && !info.Mode().IsRegular() {
		EnvExitln("indexFile must be a regular file")
	}
}

func (h *staticHandlet) Handle(req Request, resp Response) (handled bool) {
	if req.MethodCode()&(MethodGET|MethodHEAD) == 0 {
		resp.SendMethodNotAllowed("GET, HEAD", nil)
		return true
	}

	var fullPath []byte
	var pathSize int
	if h.useAppWebRoot {
		fullPath = req.unsafeAbsPath()
		pathSize = len(fullPath)
	} else { // custom web root
		userPath := req.UnsafePath()
		fullPath = req.UnsafeMake(len(h.webRoot) + len(userPath) + len(h.indexFile))
		pathSize = copy(fullPath, h.webRoot)
		pathSize += copy(fullPath[pathSize:], userPath)
	}
	isFile := fullPath[pathSize-1] != '/'
	var openPath []byte
	if isFile {
		openPath = fullPath[:pathSize]
	} else { // is directory, add indexFile to openPath
		if h.useAppWebRoot {
			openPath = req.UnsafeMake(len(fullPath) + len(h.indexFile))
			copy(openPath, fullPath)
			copy(openPath[pathSize:], h.indexFile)
			fullPath = openPath
		} else { // custom web root
			openPath = fullPath
		}
	}

	fcache := h.stage.Fcache()
	entry, err := fcache.getEntry(openPath)
	if err != nil { // entry does not exist
		if DebugLevel() >= 1 {
			Println("entry MISS")
		}
		if entry, err = fcache.newEntry(string(openPath)); err != nil {
			if !os.IsNotExist(err) {
				h.webapp.Logf("open file error=%s\n", err.Error())
				resp.SendInternalServerError(nil)
			} else if isFile { // file not found
				resp.SendNotFound(h.webapp.text404)
			} else if h.autoIndex { // index file not found, but auto index is turned on, try list directory
				if dir, err := os.Open(WeakString(fullPath[:pathSize])); err == nil {
					h.listDir(dir, resp)
					dir.Close()
				} else if !os.IsNotExist(err) {
					h.webapp.Logf("open dir error=%s\n", err.Error())
					resp.SendInternalServerError(nil)
				} else { // directory not found
					resp.SendNotFound(h.webapp.text404)
				}
			} else { // not auto index
				resp.SendForbidden(nil)
			}
			return true
		}
	}
	if entry.isDir() {
		resp.SetStatus(StatusFound)
		resp.AddDirectoryRedirection()
		resp.SendBytes(nil)
		return true
	}

	if entry.isLarge() {
		defer entry.decRef()
	}

	date := entry.info.ModTime().Unix()
	size := entry.info.Size()
	etag, _ := resp.MakeETagFrom(date, size) // with ""
	const asOrigin = true
	if status, normal := req.EvalPreconditions(date, etag, asOrigin); !normal { // not modified, or precondition failed
		resp.SetStatus(status)
		if status == StatusNotModified {
			resp.AddHeaderBytes(bytesETag, etag)
		}
		resp.SendBytes(nil)
		return true
	}
	contentType := h.defaultType
	filePath := WeakString(openPath)
	if p := strings.LastIndex(filePath, "."); p >= 0 {
		ext := filePath[p+1:]
		if mimeType, ok := h.mimeTypes[ext]; ok {
			contentType = mimeType
		}
	}
	if !req.HasRanges() || (req.HasIfRange() && !req.EvalIfRange(date, etag, asOrigin)) {
		resp.AddHeaderBytes(bytesContentType, ConstBytes(contentType))
		resp.AddHeaderBytes(bytesAcceptRanges, bytesBytes)
		if DebugLevel() >= 2 { // TODO
			resp.AddHeaderBytes(bytesCacheControl, []byte("no-cache, no-store, must-revalidate"))
		} else {
			resp.AddHeaderBytes(bytesETag, etag)
			resp.SetLastModified(date)
		}
	} else if ranges := req.EvalRanges(size); ranges != nil { // ranges are satisfiable
		resp.pickRanges(ranges, contentType)
	} else { // ranges are not satisfiable
		resp.SendRangeNotSatisfiable(size, nil)
		return true
	}
	if entry.isSmall() {
		resp.sendText(entry.text)
	} else {
		resp.sendFile(entry.file, entry.info, false) // false means don't close on end. this file belongs to fcache
	}
	return true
}

func (h *staticHandlet) listDir(dir *os.File, resp Response) {
	fis, err := dir.Readdir(-1)
	if err != nil {
		resp.SendInternalServerError([]byte("Internal Server Error 5"))
		return
	}
	resp.Echo(`<table border="1">`)
	resp.Echo(`<tr><th>name</th><th>size(in bytes)</th><th>time</th></tr>`)
	for _, fi := range fis {
		name := fi.Name()
		size := strconv.FormatInt(fi.Size(), 10)
		date := fi.ModTime().String()
		line := `<tr><td><a href="` + staticEscape(name) + `">` + staticEscape(name) + `</a></td><td>` + size + `</td><td>` + date + `</td></tr>`
		resp.Echo(line)
	}
	resp.Echo("</table>")
}

func staticEscape(s string) string { return staticEscaper.Replace(s) }

var staticEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

var staticDefaultMimeTypes = map[string]string{
	"7z":   "application/x-7z-compressed",
	"atom": "application/atom+xml",
	"bin":  "application/octet-stream",
	"bmp":  "image/x-ms-bmp",
	"css":  "text/css",
	"deb":  "application/octet-stream",
	"dll":  "application/octet-stream",
	"doc":  "application/msword",
	"dmg":  "application/octet-stream",
	"exe":  "application/octet-stream",
	"flv":  "video/x-flv",
	"gif":  "image/gif",
	"htm":  "text/html",
	"html": "text/html",
	"ico":  "image/x-icon",
	"img":  "application/octet-stream",
	"iso":  "application/octet-stream",
	"jar":  "application/java-archive",
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"js":   "application/javascript",
	"json": "application/json",
	"m4a":  "audio/x-m4a",
	"mov":  "video/quicktime",
	"mp3":  "audio/mpeg",
	"mp4":  "video/mp4",
	"mpeg": "video/mpeg",
	"mpg":  "video/mpeg",
	"pdf":  "application/pdf",
	"png":  "image/png",
	"ppt":  "application/vnd.ms-powerpoint",
	"ps":   "application/postscript",
	"rar":  "application/x-rar-compressed",
	"rss":  "application/rss+xml",
	"rtf":  "application/rtf",
	"svg":  "image/svg+xml",
	"txt":  "text/plain",
	"war":  "application/java-archive",
	"webm": "video/webm",
	"webp": "image/webp",
	"xls":  "application/vnd.ms-excel",
	"xml":  "text/xml",
	"zip":  "application/zip",
}
