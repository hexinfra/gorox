// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP server implementation. See RFC 9110.

package hemi

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// HTTPServer is the http server.
type HTTPServer interface { // for *http[x3]Server
	// Imports
	Server
	// Methods
	MaxConcurrentConnsPerGate() int32
	bindWebapps()
	httpHolder() _httpHolder_ // used by gates to copy the configs
}

// httpServer_ is a parent.
type httpServer_[G httpGate] struct { // for http[x3]Server
	// Parent
	Server_[G]
	// Mixins
	_httpHolder_ // to carry configs used by gates
	// Assocs
	defaultWebapp *Webapp // default webapp if not found
	// States
	webapps                   []string               // for what webapps
	exactWebapps              []*hostnameTo[*Webapp] // like: ("example.com")
	suffixWebapps             []*hostnameTo[*Webapp] // like: ("*.example.com")
	prefixWebapps             []*hostnameTo[*Webapp] // like: ("www.example.*")
	forceScheme               int8                   // scheme (http/https) that must be used
	alignScheme               bool                   // if scheme is not forced, use https scheme for TLS and http scheme for others?
	maxConcurrentConnsPerGate int32                  // max concurrent connections allowed per gate
}

func (s *httpServer_[G]) onCreate(compName string, stage *Stage) {
	s.Server_.OnCreate(compName, stage)

	s.forceScheme = -1 // not forced
}

func (s *httpServer_[G]) onConfigure() {
	s.Server_.OnConfigure()
	s._httpHolder_.onConfigure(s, 0*time.Second, 0*time.Second, TmpDir()+"/web/servers/"+s.compName)

	// .webapps
	s.ConfigureStringList("webapps", &s.webapps, nil, []string{})

	// .forceScheme
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

	// .alignScheme
	s.ConfigureBool("alignScheme", &s.alignScheme, true)

	// .maxConcurrentConnsPerGate
	s.ConfigureInt32("maxConcurrentConnsPerGate", &s.maxConcurrentConnsPerGate, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxConcurrentConnsPerGate has an invalid value")
	}, 10000)
}
func (s *httpServer_[G]) onPrepare() {
	s.Server_.OnPrepare()
	s._httpHolder_.onPrepare(s, 0755)
}

func (s *httpServer_[G]) MaxConcurrentConnsPerGate() int32 { return s.maxConcurrentConnsPerGate }

func (s *httpServer_[G]) bindWebapps() {
	for _, webappName := range s.webapps {
		webapp := s.stage.Webapp(webappName)
		if webapp == nil {
			continue
		}
		if s.TLSMode() {
			if webapp.tlsCertificate == "" || webapp.tlsPrivateKey == "" {
				UseExitln("webapps that bound to tls server must have certificates and private keys")
			}
			certificate, err := tls.LoadX509KeyPair(webapp.tlsCertificate, webapp.tlsPrivateKey)
			if err != nil {
				UseExitln(err.Error())
			}
			if DebugLevel() >= 1 {
				Printf("adding certificate to %s\n", s.Colonport())
			}
			s.tlsConfig.Certificates = append(s.tlsConfig.Certificates, certificate)
		}
		webapp.bindServer(s.shell.(HTTPServer))
		if webapp.isDefault {
			s.defaultWebapp = webapp
		}
		// TODO: use hash table?
		for _, hostname := range webapp.exactHostnames {
			s.exactWebapps = append(s.exactWebapps, &hostnameTo[*Webapp]{hostname, webapp})
		}
		// TODO: use radix trie?
		for _, hostname := range webapp.suffixHostnames {
			s.suffixWebapps = append(s.suffixWebapps, &hostnameTo[*Webapp]{hostname, webapp})
		}
		// TODO: use radix trie?
		for _, hostname := range webapp.prefixHostnames {
			s.prefixWebapps = append(s.prefixWebapps, &hostnameTo[*Webapp]{hostname, webapp})
		}
	}
}
func (s *httpServer_[G]) findWebapp(hostname []byte) *Webapp {
	// TODO: use hash table?
	for _, exactMap := range s.exactWebapps {
		if bytes.Equal(hostname, exactMap.hostname) {
			return exactMap.target
		}
	}
	// TODO: use radix trie?
	for _, suffixMap := range s.suffixWebapps {
		if bytes.HasSuffix(hostname, suffixMap.hostname) {
			return suffixMap.target
		}
	}
	// TODO: use radix trie?
	for _, prefixMap := range s.prefixWebapps {
		if bytes.HasPrefix(hostname, prefixMap.hostname) {
			return prefixMap.target
		}
	}
	return s.defaultWebapp // may be nil
}

func (s *httpServer_[G]) httpHolder() _httpHolder_ { return s._httpHolder_ } // for copying configs

// httpGate is the http gate.
type httpGate interface { // for *http[x3]Gate
	// Imports
	Gate
	httpHolder
	// Methods
}

// httpGate_ is a parent.
type httpGate_[S HTTPServer] struct { // for http[x3]Gate
	// Parent
	Gate_[S]
	// Mixins
	_httpHolder_ // holds conns
	// States
	maxConcurrentConns int32        // max concurrent conns allowed for this gate
	concurrentConns    atomic.Int32 // current concurrent conns. TODO: false sharing
}

func (g *httpGate_[S]) onNew(server S, id int32) {
	g.Gate_.OnNew(server, id)
	g._httpHolder_ = server.httpHolder()
	g.maxConcurrentConns = server.MaxConcurrentConnsPerGate()
	g.concurrentConns.Store(0)
}

func (g *httpGate_[S]) DecConcurrentConns() int32 { return g.concurrentConns.Add(-1) }
func (g *httpGate_[S]) IncConcurrentConns() int32 { return g.concurrentConns.Add(1) }
func (g *httpGate_[S]) ReachLimit(concurrentConns int32) bool {
	return concurrentConns > g.maxConcurrentConns
}

// serverConn is the server-side http connection.
type serverConn interface { // for *server[1-3]Conn
}

// _serverConn_ is a mixin.
type _serverConn_[G httpGate] struct { // for server[1-3]Conn
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	gate G // the gate to which the conn belongs
	// Conn states (zeros)
}

func (c *_serverConn_[G]) onGet(gate G) {
	c.gate = gate
}
func (c *_serverConn_[G]) onPut() {
	var null G // nil
	c.gate = null
}

func (c *_serverConn_[G]) Holder() httpHolder { return c.gate }

// serverStream is the server-side http stream.
type serverStream interface { // for *server[1-3]Stream
}

// _serverStream_ is a mixin.
type _serverStream_ struct { // for server[1-3]Stream
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *_serverStream_) onUse() {
}
func (s *_serverStream_) onEnd() {
}

// ServerRequest is the server-side http request.
type ServerRequest interface { // for *server[1-3]Request
	RemoteAddr() net.Addr
	Webapp() *Webapp

	IsAsteriskOptions() bool // OPTIONS *

	VersionCode() uint8
	IsHTTP1() bool
	IsHTTP1_0() bool
	IsHTTP1_1() bool
	IsHTTP2() bool
	IsHTTP3() bool
	Version() string // HTTP/1.0, HTTP/1.1, HTTP/2, HTTP/3
	RiskyVersion() []byte

	SchemeCode() uint8 // SchemeHTTP, SchemeHTTPS
	IsHTTP() bool
	IsHTTPS() bool
	Scheme() string // http, https
	RiskyScheme() []byte

	IsGET() bool
	IsHEAD() bool
	IsPOST() bool
	IsPUT() bool
	IsDELETE() bool
	IsCONNECT() bool
	IsOPTIONS() bool
	IsTRACE() bool
	Method() string // GET, POST, ...
	RiskyMethod() []byte

	Authority() string      // hostname[:port]
	RiskyAuthority() []byte // hostname[:port]
	Hostname() string       // hostname
	RiskyHostname() []byte  // hostname
	Colonport() string      // :port
	RiskyColonport() []byte // :port

	URI() string              // /encodedPath?queryString
	RiskyURI() []byte         // /encodedPath?queryString
	Path() string             // /decodedPath
	RiskyPath() []byte        // /decodedPath
	EncodedPath() string      // /encodedPath
	RiskyEncodedPath() []byte // /encodedPath
	QueryString() string      // including '?' if query string exists, otherwise empty
	RiskyQueryString() []byte // including '?' if query string exists, otherwise empty

	HasQueries() bool
	AllQueries() (queries [][2]string)
	Q(name string) string
	Qstr(name string, defaultValue string) string
	Qint(name string, defaultValue int) int
	Query(name string) (value string, ok bool)
	RiskyQuery(name string) (value []byte, ok bool)
	Queries(name string) (values []string, ok bool)
	HasQuery(name string) bool
	DelQuery(name string) (deleted bool)
	AddQuery(name string, value string) bool

	HasHeaders() bool
	AllHeaderLines() (headerLines [][2]string)
	H(name string) string
	Hstr(name string, defaultValue string) string
	Hint(name string, defaultValue int) int
	Header(name string) (value string, ok bool)
	RiskyHeader(name string) (value []byte, ok bool)
	Headers(name string) (values []string, ok bool)
	HasHeader(name string) bool
	DelHeader(name string) (deleted bool)
	AddHeader(name string, value string) bool

	UserAgent() string
	RiskyUserAgent() []byte

	ContentType() string
	RiskyContentType() []byte

	ContentIsEncoded() bool

	ContentSize() int64
	RiskyContentLength() []byte

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
	RiskyCookie(name string) (value []byte, ok bool)
	Cookies(name string) (values []string, ok bool)
	HasCookie(name string) bool
	DelCookie(name string) (deleted bool)
	AddCookie(name string, value string) bool

	SetRecvTimeout(timeout time.Duration) // to defend against slowloris attack
	HasContent() bool                     // true if content exists
	IsVague() bool                        // true if content exists and is not sized
	Content() string
	RiskyContent() []byte

	HasForms() bool
	AllForms() (forms [][2]string)
	F(name string) string
	Fstr(name string, defaultValue string) string
	Fint(name string, defaultValue int) int
	Form(name string) (value string, ok bool)
	RiskyForm(name string) (value []byte, ok bool)
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
	AllTrailerLines() (trailerLines [][2]string)
	T(name string) string
	Tstr(name string, defaultValue string) string
	Tint(name string, defaultValue int) int
	Trailer(name string) (value string, ok bool)
	RiskyTrailer(name string) (value []byte, ok bool)
	Trailers(name string) (values []string, ok bool)
	HasTrailer(name string) bool
	DelTrailer(name string) (deleted bool)
	AddTrailer(name string, value string) bool

	RiskyMake(size int) []byte

	// Internal only
	getPathInfo() os.FileInfo
	riskyAbsPath() []byte
	makeAbsPath()
	contentIsForm() bool
	proxyDelHopHeaderFields()
	proxyDelHopTrailerFields()
	proxyDelHopFieldLines(kind int8)
	proxyWalkHeaderLines(out httpOut, callback func(out httpOut, headerLine *pair, headerName []byte, lineValue []byte) bool) bool
	proxyWalkHeaderFields(callback func(headerName []byte, headerValue []byte) bool) bool // used by cgi-based protocols like cgi and fcgi
	proxyWalkTrailerLines(out httpOut, callback func(out httpOut, trailerLine *pair, trailerName []byte, lineValue []byte) bool) bool
	proxyWalkCookies(callback func(cookie *pair, cookieName []byte, cookieValue []byte) bool) bool
	proxyUnsetHost()
	proxyTakeContent() any
	readContent() (data []byte, err error)
	examineTail() bool
	hookReviser(reviser Reviser)
	riskyVariable(varCode int16, varName string) (varValue []byte)
}

// serverRequest_ is a parent.
type serverRequest_ struct { // for server[1-3]Request. incoming, needs parsing
	// Mixins
	_httpIn_ // incoming http request
	// Stream states (stocks)
	stockUpfiles [2]Upfile // for r.upfiles. 96B
	// Stream states (controlled)
	ranges [4]Range // parsed range fields. at most 4 range fields are allowed. controlled by r.numRanges
	// Stream states (non-zeros)
	upfiles []Upfile // decoded upfiles -> r.array (for metadata) and temp files in local file system. [<r.stockUpfiles>/(make=16/128)]
	// Stream states (zeros)
	webapp          *Webapp     // target webapp of this request. set before executing the stream
	path            []byte      // decoded path. only a reference. refers to r.array or region if rewrited, so can't be a span
	absPath         []byte      // webapp.webRoot + r.RiskyPath(). if webapp.webRoot is not set then this is nil. set when dispatching to handlets. only a reference
	pathInfo        os.FileInfo // cached result of os.Stat(r.absPath) if r.absPath is not nil
	formWindow      []byte      // a window used for reading and parsing content as multipart/form-data. [<none>/r.contentText/4K/16K]
	_serverRequest0             // all values in this struct must be zero by default!
}
type _serverRequest0 struct { // for fast reset, entirely
	gotSomeInput    bool     // got some input from client? for request timeout handling
	_               byte     // padding
	asteriskOptions bool     // true if method and uri is: OPTIONS *
	schemeCode      uint8    // SchemeHTTP, SchemeHTTPS
	methodCode      uint32   // known method code. 0: unknown method
	method          span     // raw method -> r.input
	authority       span     // raw hostname[:port] -> r.input
	hostname        span     // raw hostname (without :port) -> r.input
	colonport       span     // raw colon port (:port, with ':') -> r.input
	uri             span     // raw uri (raw path & raw query string) -> r.input
	encodedPath     span     // raw path -> r.input
	queryString     span     // raw query string (with '?') -> r.input
	boundary        span     // boundary parameter of "multipart/form-data" if exists -> r.input
	queries         zone     // decoded queries -> r.array
	cookies         zone     // cookies ->r.input. temporarily used when checking the cookie header, set after cookie header is parsed
	forms           zone     // decoded forms -> r.array
	ifMatch         int8     // -1: if-match *, 0: no if-match field, >0: number of if-match: 1#entity-tag
	ifNoneMatch     int8     // -1: if-none-match *, 0: no if-none-match field, >0: number of if-none-match: 1#entity-tag
	numRanges       int8     // num of ranges. controls r.ranges
	maxForwards     int8     // parsed value of "Max-Forwards" header field, must <= 127
	expectContinue  bool     // expect: 100-continue?
	acceptTrailers  bool     // does client accept trailer section? i.e. te: trailers
	pathInfoGot     bool     // is r.pathInfo got?
	_               [3]byte  // padding
	indexes         struct { // indexes of some selected singleton header fields, for fast accessing
		authorization      uint8   // authorization header field ->r.input
		host               uint8   // host header field ->r.input
		ifModifiedSince    uint8   // if-modified-since header field ->r.input
		ifRange            uint8   // if-range header field ->r.input
		ifUnmodifiedSince  uint8   // if-unmodified-since header field ->r.input
		maxForwards        uint8   // max-forwards header field ->r.input
		proxyAuthorization uint8   // proxy-authorization header field ->r.input
		referer            uint8   // referer header field ->r.input
		userAgent          uint8   // user-agent header field ->r.input
		_                  [7]byte // padding
	}
	zones struct { // zones (may not be continuous) of some selected important header fields, for fast accessing
		acceptLanguage  zone    // the zone of accept-language in r.primes
		expect          zone    // the zone of expect in r.primes
		forwarded       zone    // the zone of forwarded in r.primes
		ifMatch         zone    // the zone of if-match in r.primes
		ifNoneMatch     zone    // the zone of if-none-match in r.primes
		te              zone    // the zone of te in r.primes
		xForwardedBy    zone    // the zone of x-forwarded-by in r.primes
		xForwardedFor   zone    // the zone of x-forwarded-for in r.primes
		xForwardedHost  zone    // the zone of x-forwarded-host in r.primes
		xForwardedProto zone    // the zone of x-forwarded-proto in r.primes
		_               [4]byte // padding
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
	r._httpIn_.onUse(httpVersion, false) // as request

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
			filePath = string(r.array[upfile.pathFrom : upfile.pathFrom+int32(upfile.pathSize)])
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
	r._serverRequest0 = _serverRequest0{}

	r._httpIn_.onEnd()
}

func (r *serverRequest_) Webapp() *Webapp { return r.webapp }

func (r *serverRequest_) IsAsteriskOptions() bool { return r.asteriskOptions }

func (r *serverRequest_) SchemeCode() uint8   { return r.schemeCode }
func (r *serverRequest_) IsHTTP() bool        { return r.schemeCode == SchemeHTTP }
func (r *serverRequest_) IsHTTPS() bool       { return r.schemeCode == SchemeHTTPS }
func (r *serverRequest_) Scheme() string      { return httpSchemeStrings[r.schemeCode] }
func (r *serverRequest_) RiskyScheme() []byte { return httpSchemeByteses[r.schemeCode] }

func (r *serverRequest_) IsGET() bool         { return r.methodCode == MethodGET }
func (r *serverRequest_) IsHEAD() bool        { return r.methodCode == MethodHEAD }
func (r *serverRequest_) IsPOST() bool        { return r.methodCode == MethodPOST }
func (r *serverRequest_) IsPUT() bool         { return r.methodCode == MethodPUT }
func (r *serverRequest_) IsDELETE() bool      { return r.methodCode == MethodDELETE }
func (r *serverRequest_) IsCONNECT() bool     { return r.methodCode == MethodCONNECT }
func (r *serverRequest_) IsOPTIONS() bool     { return r.methodCode == MethodOPTIONS }
func (r *serverRequest_) IsTRACE() bool       { return r.methodCode == MethodTRACE }
func (r *serverRequest_) Method() string      { return string(r.RiskyMethod()) }
func (r *serverRequest_) RiskyMethod() []byte { return r.input[r.method.from:r.method.edge] }
func (r *serverRequest_) recognizeMethod(method []byte, methodHash uint16) {
	if m := serverMethodTable[serverMethodFind(methodHash)]; m.hash == methodHash && bytes.Equal(serverMethodBytes[m.from:m.edge], method) {
		r.methodCode = m.code
	}
}

var ( // minimal perfect hash table for best known http methods
	serverMethodBytes = []byte("GET HEAD POST PUT DELETE CONNECT OPTIONS TRACE")
	serverMethodTable = [8]struct {
		hash uint16
		from uint8
		edge uint8
		code uint32
	}{
		0: {326, 9, 13, MethodPOST},
		1: {274, 4, 8, MethodHEAD},
		2: {249, 14, 17, MethodPUT},
		3: {224, 0, 3, MethodGET},
		4: {556, 33, 40, MethodOPTIONS},
		5: {522, 25, 32, MethodCONNECT},
		6: {435, 18, 24, MethodDELETE},
		7: {367, 41, 46, MethodTRACE},
	}
	serverMethodFind = func(methodHash uint16) int { return (2610 / int(methodHash)) % len(serverMethodTable) }
)

func (r *serverRequest_) Authority() string { return string(r.RiskyAuthority()) }
func (r *serverRequest_) RiskyAuthority() []byte {
	return r.input[r.authority.from:r.authority.edge]
}
func (r *serverRequest_) Hostname() string      { return string(r.RiskyHostname()) }
func (r *serverRequest_) RiskyHostname() []byte { return r.input[r.hostname.from:r.hostname.edge] }
func (r *serverRequest_) Colonport() string {
	if r.colonport.notEmpty() {
		return string(r.input[r.colonport.from:r.colonport.edge])
	}
	if r.schemeCode == SchemeHTTPS {
		return stringColonport443
	} else {
		return stringColonport80
	}
}
func (r *serverRequest_) RiskyColonport() []byte {
	if r.colonport.notEmpty() {
		return r.input[r.colonport.from:r.colonport.edge]
	}
	if r.schemeCode == SchemeHTTPS {
		return bytesColonport443
	} else {
		return bytesColonport80
	}
}

func (r *serverRequest_) URI() string {
	if r.uri.notEmpty() {
		return string(r.input[r.uri.from:r.uri.edge])
	} else { // use "/"
		return stringSlash
	}
}
func (r *serverRequest_) RiskyURI() []byte {
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
func (r *serverRequest_) RiskyEncodedPath() []byte {
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
func (r *serverRequest_) RiskyPath() []byte {
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
func (r *serverRequest_) riskyAbsPath() []byte { return r.absPath }
func (r *serverRequest_) makeAbsPath() {
	if r.webapp.webRoot == "" { // if webapp's webRoot is empty, r.absPath is not used either. so it's safe to do nothing
		return
	}
	webRoot := r.webapp.webRoot
	r.absPath = r.RiskyMake(len(webRoot) + len(r.RiskyPath()))
	n := copy(r.absPath, webRoot)
	copy(r.absPath[n:], r.RiskyPath())
}
func (r *serverRequest_) getPathInfo() os.FileInfo {
	if !r.pathInfoGot {
		r.pathInfoGot = true
		if pathInfo, err := os.Stat(string(r.absPath)); err == nil {
			r.pathInfo = pathInfo
		}
	}
	return r.pathInfo
}
func (r *serverRequest_) QueryString() string { return string(r.RiskyQueryString()) }
func (r *serverRequest_) RiskyQueryString() []byte {
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
func (r *serverRequest_) RiskyQuery(name string) (value []byte, ok bool) {
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
func (r *serverRequest_) AddQuery(name string, value string) bool { // as extra, by webapp
	return r.addExtra(name, value, 0, pairQuery)
}

func (r *serverRequest_) examineHead() bool {
	for i := r.headerLines.from; i < r.headerLines.edge; i++ {
		if !r._applyHeaderLine(i) {
			// r.headResult is set.
			return false
		}
	}
	if r.cookies.notEmpty() { // in HTTP/2 and HTTP/3, there can be multiple cookie fields.
		cookies := r.cookies // make a copy, as r.cookies will be changed as cookie pairs below
		r.cookies.from = uint8(len(r.primes))
		for i := cookies.from; i < cookies.edge; i++ {
			cookie := &r.primes[i]
			if cookie.nameHash != hashCookie || !cookie.nameEqualBytes(r.input, bytesCookie) { // cookies may not be consecutive in http/2 and http/3
				continue
			}
			if !r.parseCookie(cookie.value) { // r.cookies.edge is set in r.addCookie().
				return false
			}
		}
	}
	if DebugLevel() >= 3 {
		Println("======primes======")
		for i := range len(r.primes) {
			prime := &r.primes[i]
			prime.show(r._placeOf(prime))
		}
		Println("======extras======")
		for i := range len(r.extras) {
			extra := &r.extras[i]
			extra.show(r._placeOf(extra))
		}
	}

	// RFC 9110 (section 5.3):
	// A server MUST NOT apply a request to the target resource until it receives the entire request header section,
	// since later header field lines might include conditionals, authentication credentials,
	// or deliberately misleading duplicate header fields that could impact request processing.

	// Basic checks against versions
	switch r.httpVersion {
	case Version1_0:
		if r.keepAlive == -1 { // no connection header field
			r.keepAlive = 0 // default is close for HTTP/1.0
		}
	case Version1_1:
		if r.keepAlive == -1 { // no connection header field
			r.keepAlive = 1 // default is keep-alive for HTTP/1.1
		}
		if r.indexes.host == 0 {
			// RFC 9112 (section 3.2):
			// A server MUST respond with a 400 (Bad Request) status code to any HTTP/1.1 request message that lacks a Host header field.
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
		if !r.IsGET() || r.httpVersion == Version1_0 || r.contentSize != -1 {
			r.headResult, r.failReason = StatusMethodNotAllowed, "webSocket only supports GET method and HTTP version >= 1.1, without content"
			return false
		}
	}
	if r.methodCode&(MethodCONNECT|MethodOPTIONS|MethodTRACE) != 0 {
		// RFC 9110 (section 13.2.1):
		// Likewise, a server MUST ignore the conditional request header
		// fields defined by this specification when received with a request
		// method that does not involve the selection or modification of a
		// selected representation, such as CONNECT, OPTIONS, or TRACE.
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
		if r.indexes.ifRange != 0 && r.numRanges == 0 {
			r._delPrime(r.indexes.ifRange) // we delete it.
			r.indexes.ifRange = 0
		}
	}
	if r.contentSize == -1 { // no content
		if r.expectContinue { // expect is used to send large content.
			r.headResult, r.failReason = StatusBadRequest, "cannot use expect header field without content"
			return false
		}
		if r.methodCode&(MethodPOST|MethodPUT) != 0 {
			r.headResult, r.failReason = StatusLengthRequired, "POST and PUT must contain a content"
			return false
		}
	} else { // content exists (sized or vague)
		// Content is not allowed in some methods, according to RFC 9110.
		if r.methodCode&(MethodCONNECT|MethodTRACE) != 0 {
			r.headResult, r.failReason = StatusBadRequest, "content is not allowed in CONNECT and TRACE method"
			return false
		}
		if r.iContentType == 0 { // no content-type
			if r.IsOPTIONS() {
				// RFC 9110 (section 9.3.7):
				// A client that generates an OPTIONS request containing content MUST send
				// a valid Content-Type header field describing the representation media type.
				r.headResult, r.failReason = StatusBadRequest, "OPTIONS with content but without a content-type"
				return false
			}
		} else { // has content-type
			headerLine := &r.primes[r.iContentType]
			contentType := headerLine.dataAt(r.input)
			bytesToLower(contentType)
			if bytes.Equal(contentType, bytesURLEncodedForm) {
				r.formKind = httpFormURLEncoded
			} else if bytes.Equal(contentType, bytesMultipartForm) { // multipart/form-data; boundary=xxxxxx
				for i := headerLine.params.from; i < headerLine.params.edge; i++ {
					param := &r.extras[i]
					if param.nameHash != hashBoundary || !param.nameEqualBytes(r.input, bytesBoundary) {
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
		}
	}

	return true
}
func (r *serverRequest_) _applyHeaderLine(lineIndex uint8) bool {
	headerLine := &r.primes[lineIndex]
	headerName := headerLine.nameAt(r.input)
	if sh := &serverRequestSingletonHeaderFieldTable[serverRequestSingletonHeaderFieldFind(headerLine.nameHash)]; sh.nameHash == headerLine.nameHash && bytes.Equal(sh.name, headerName) {
		headerLine.setSingleton()
		if !sh.parse { // unnecessary to parse generally
			headerLine.setParsed()
			headerLine.dataEdge = headerLine.value.edge
		} else if !r._parseFieldLine(headerLine, &sh.fdesc, r.input, true) { // fully
			r.headResult = StatusBadRequest
			return false
		}
		if !sh.check(r, headerLine, lineIndex) {
			// r.headResult is set.
			return false
		}
	} else if mh := &serverRequestImportantHeaderFieldTable[serverRequestImportantHeaderFieldFind(headerLine.nameHash)]; mh.nameHash == headerLine.nameHash && bytes.Equal(mh.name, headerName) {
		extraFrom := uint8(len(r.extras))
		if !r._splitFieldLine(headerLine, &mh.fdesc, r.input) {
			r.headResult = StatusBadRequest
			return false
		}
		if headerLine.isCommaValue() { // has sub header lines, check them
			if extraEdge := uint8(len(r.extras)); !mh.check(r, r.extras, extraFrom, extraEdge) {
				// r.headResult is set.
				return false
			}
		} else if !mh.check(r, r.primes, lineIndex, lineIndex+1) { // no sub header lines. check it
			// r.headResult is set.
			return false
		}
	} else {
		// All other header fields are treated as list-based header fields.
	}
	return true
}

var ( // minimal perfect hash table for singleton request header fields
	serverRequestSingletonHeaderFieldTable = [16]struct {
		parse bool // need general parse or not
		fdesc      // allowQuote, allowEmpty, allowParam, hasComment
		check func(*serverRequest_, *pair, uint8) bool
	}{ // authorization content-length content-location content-range content-type cookie date host if-modified-since if-range if-unmodified-since max-forwards proxy-authorization range referer user-agent
		0:  {true, fdesc{hashReferer, true, false, false, false, bytesReferer}, (*serverRequest_).checkReferer},
		1:  {false, fdesc{hashMaxForwards, false, false, false, false, bytesMaxForwards}, (*serverRequest_).checkMaxForwards},
		2:  {true, fdesc{hashContentType, false, false, true, false, bytesContentType}, (*serverRequest_).checkContentType},
		3:  {false, fdesc{hashDate, false, false, false, false, bytesDate}, (*serverRequest_).checkDate},
		4:  {false, fdesc{hashRange, false, false, false, false, bytesRange}, (*serverRequest_).checkRange},
		5:  {false, fdesc{hashContentRange, false, false, false, false, bytesContentRange}, (*serverRequest_).checkContentRange},
		6:  {false, fdesc{hashContentLength, false, false, false, false, bytesContentLength}, (*serverRequest_).checkContentLength},
		7:  {false, fdesc{hashUserAgent, false, false, false, true, bytesUserAgent}, (*serverRequest_).checkUserAgent},
		8:  {false, fdesc{hashProxyAuthorization, false, false, false, false, bytesProxyAuthorization}, (*serverRequest_).checkProxyAuthorization},
		9:  {false, fdesc{hashAuthorization, false, false, false, false, bytesAuthorization}, (*serverRequest_).checkAuthorization},
		10: {false, fdesc{hashIfRange, false, false, false, false, bytesIfRange}, (*serverRequest_).checkIfRange},
		11: {false, fdesc{hashHost, false, false, false, false, bytesHost}, (*serverRequest_).checkHost},
		12: {false, fdesc{hashIfUnmodifiedSince, false, false, false, false, bytesIfUnmodifiedSince}, (*serverRequest_).checkIfUnmodifiedSince},
		13: {false, fdesc{hashIfModifiedSince, false, false, false, false, bytesIfModifiedSince}, (*serverRequest_).checkIfModifiedSince},
		14: {true, fdesc{hashContentLocation, true, false, false, false, bytesContentLocation}, (*serverRequest_).checkContentLocation},
		15: {false, fdesc{hashCookie, false, false, false, false, bytesCookie}, (*serverRequest_).checkCookie}, // `a=b; c=d; e=f` is cookie list, not parameters
	}
	serverRequestSingletonHeaderFieldFind = func(nameHash uint16) int {
		return (8175626 / int(nameHash)) % len(serverRequestSingletonHeaderFieldTable)
	}
)

func (r *serverRequest_) checkAuthorization(headerLine *pair, lineIndex uint8) bool { // Authorization = auth-scheme [ 1*SP ( token68 / #auth-param ) ]
	// auth-scheme = token
	// token68     = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
	// auth-param  = token BWS "=" BWS ( token / quoted-string )
	// TODO
	if r.indexes.authorization != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated authorization header field"
		return false
	}
	r.indexes.authorization = lineIndex
	return true
}
func (r *serverRequest_) checkCookie(headerLine *pair, lineIndex uint8) bool { // Cookie = cookie-string
	if headerLine.value.isEmpty() {
		r.headResult, r.failReason = StatusBadRequest, "empty cookie"
		return false
	}
	if lineIndex == 255 {
		r.headResult, r.failReason = StatusBadRequest, "too many pairs"
		return false
	}
	// HTTP/2 and HTTP/3 allows multiple cookie header fields, so we have to mark all the cookie header fields.
	if r.cookies.isEmpty() {
		r.cookies.from = lineIndex
	}
	// And we can't inject cookies into header lines zone while receiving header lines, this will break the continuous nature of header lines zone.
	r.cookies.edge = lineIndex + 1 // so we postpone cookie parsing after the request head is entirely received. only mark the edge
	return true
}
func (r *serverRequest_) checkHost(headerLine *pair, lineIndex uint8) bool { // Host = uri-host [ ":" port ]
	// RFC 9112 (section 3.2):
	// A server MUST respond with a 400 (Bad Request) status code to any HTTP/1.1 request message that lacks a Host header field and
	// to any request message that contains more than one Host header field line or a Host header field with an invalid field value.
	if r.indexes.host != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicate host header field"
		return false
	}
	host := headerLine.value
	if host.notEmpty() {
		// RFC 9110 (section 4.2.3):
		// The scheme and host are case-insensitive and normally provided in lowercase;
		// all other components are compared in a case-sensitive manner.
		bytesToLower(r.input[host.from:host.edge])
		if !r.parseAuthority(host.from, host.edge, r.authority.isEmpty()) {
			r.headResult, r.failReason = StatusBadRequest, "bad host value"
			return false
		}
	}
	r.indexes.host = lineIndex
	return true
}
func (r *serverRequest_) checkIfModifiedSince(headerLine *pair, lineIndex uint8) bool { // If-Modified-Since = HTTP-date
	return r._checkHTTPDate(headerLine, lineIndex, &r.indexes.ifModifiedSince, &r.unixTimes.ifModifiedSince)
}
func (r *serverRequest_) checkIfRange(headerLine *pair, lineIndex uint8) bool { // If-Range = entity-tag / HTTP-date
	if r.indexes.ifRange != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated if-range header field"
		return false
	}
	if date, ok := clockParseHTTPDate(headerLine.valueAt(r.input)); ok {
		r.unixTimes.ifRange = date
	}
	r.indexes.ifRange = lineIndex
	return true
}
func (r *serverRequest_) checkIfUnmodifiedSince(headerLine *pair, lineIndex uint8) bool { // If-Unmodified-Since = HTTP-date
	return r._checkHTTPDate(headerLine, lineIndex, &r.indexes.ifUnmodifiedSince, &r.unixTimes.ifUnmodifiedSince)
}
func (r *serverRequest_) checkMaxForwards(headerLine *pair, lineIndex uint8) bool { // Max-Forwards = Max-Forwards = 1*DIGIT
	if r.indexes.maxForwards != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated max-forwards header field"
		return false
	}
	// TODO: parse headerLine.valueAt(r.input) as 1*DIGIT into r.maxForwards
	r.indexes.maxForwards = lineIndex
	return true
}
func (r *serverRequest_) checkProxyAuthorization(headerLine *pair, lineIndex uint8) bool { // Proxy-Authorization = auth-scheme [ 1*SP ( token68 / #auth-param ) ]
	// auth-scheme = token
	// token68     = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
	// auth-param  = token BWS "=" BWS ( token / quoted-string )
	if r.indexes.proxyAuthorization != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated proxyAuthorization header field"
		return false
	}
	// TODO: check
	r.indexes.proxyAuthorization = lineIndex
	return true
}
func (r *serverRequest_) checkRange(headerLine *pair, lineIndex uint8) bool { // Range = ranges-specifier
	if !r.IsGET() {
		// A server MUST ignore a Range header field received with a request method that is unrecognized or for which range handling is not defined.
		// For this specification, GET is the only method for which range handling is defined.
		r._delPrime(lineIndex)
		return true
	}
	if r.numRanges > 0 { // we have already got a valid range header field
		r.headResult, r.failReason = StatusBadRequest, "duplicated range header field"
		return false
	}
	// Range        = range-unit "=" range-set
	// range-set    = 1#range-spec
	// range-spec   = int-range / suffix-range
	// int-range    = first-pos "-" [ last-pos ]
	// suffix-range = "-" suffix-length
	rangeSet := headerLine.valueAt(r.input)
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
func (r *serverRequest_) checkReferer(headerLine *pair, lineIndex uint8) bool { // Referer = absolute-URI / partial-URI
	if r.indexes.referer != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated referer header field"
		return false
	}
	r.indexes.referer = lineIndex
	return true
}
func (r *serverRequest_) checkUserAgent(headerLine *pair, lineIndex uint8) bool { // User-Agent = product *( RWS ( product / comment ) )
	if r.indexes.userAgent != 0 {
		r.headResult, r.failReason = StatusBadRequest, "duplicated user-agent header field"
		return false
	}
	r.indexes.userAgent = lineIndex
	return true
}
func (r *serverRequest_) _addRange(rang Range) bool {
	if r.numRanges == int8(cap(r.ranges)) { // possible attack
		r.headResult, r.failReason = StatusBadRequest, "too many ranges"
		return false
	}
	r.ranges[r.numRanges] = rang
	r.numRanges++
	return true
}

var ( // minimal perfect hash table for important request header fields
	serverRequestImportantHeaderFieldTable = [22]struct {
		fdesc // allowQuote, allowEmpty, allowParam, hasComment
		check func(*serverRequest_, []pair, uint8, uint8) bool
	}{ // accept accept-encoding accept-language cache-control connection content-encoding content-language expect forwarded if-match if-none-match keep-alive proxy-connection te trailer transfer-encoding upgrade via x-forwarded-by x-forwarded-for x-forwarded-host x-forwarded-proto
		0:  {fdesc{hashConnection, false, false, false, false, bytesConnection}, (*serverRequest_).checkConnection},
		1:  {fdesc{hashProxyConnection, false, false, false, false, bytesProxyConnection}, (*serverRequest_).checkProxyConnection},
		2:  {fdesc{hashXForwardedHost, false, false, false, false, bytesXForwardedHost}, (*serverRequest_).checkXForwardedHost},
		3:  {fdesc{hashAccept, false, true, true, false, bytesAccept}, (*serverRequest_).checkAccept},
		4:  {fdesc{hashIfNoneMatch, true, false, false, false, bytesIfNoneMatch}, (*serverRequest_).checkIfNoneMatch},
		5:  {fdesc{hashXForwardedProto, false, false, false, false, bytesXForwardedProto}, (*serverRequest_).checkXForwardedProto},
		6:  {fdesc{hashTE, false, false, true, false, bytesTE}, (*serverRequest_).checkTE},
		7:  {fdesc{hashContentLanguage, false, false, false, false, bytesContentLanguage}, (*serverRequest_).checkContentLanguage},
		8:  {fdesc{hashTrailer, false, false, false, false, bytesTrailer}, (*serverRequest_).checkTrailer},
		9:  {fdesc{hashForwarded, false, false, false, false, bytesForwarded}, (*serverRequest_).checkForwarded}, // note: `for=192.0.2.60;proto=http;by=203.0.113.43` is not parameters
		10: {fdesc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*serverRequest_).checkUpgrade},
		11: {fdesc{hashKeepAlive, false, false, false, false, bytesKeepAlive}, (*serverRequest_).checkKeepAlive},
		12: {fdesc{hashAcceptEncoding, false, true, true, false, bytesAcceptEncoding}, (*serverRequest_).checkAcceptEncoding},
		13: {fdesc{hashContentEncoding, false, false, false, false, bytesContentEncoding}, (*serverRequest_).checkContentEncoding},
		14: {fdesc{hashVia, false, false, false, true, bytesVia}, (*serverRequest_).checkVia},
		15: {fdesc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*serverRequest_).checkTransferEncoding}, // deliberately false
		16: {fdesc{hashIfMatch, true, false, false, false, bytesIfMatch}, (*serverRequest_).checkIfMatch},
		17: {fdesc{hashXForwardedBy, false, false, false, false, bytesXForwardedBy}, (*serverRequest_).checkXForwardedBy},
		18: {fdesc{hashXForwardedFor, false, false, false, false, bytesXForwardedFor}, (*serverRequest_).checkXForwardedFor},
		19: {fdesc{hashAcceptLanguage, false, false, true, false, bytesAcceptLanguage}, (*serverRequest_).checkAcceptLanguage},
		20: {fdesc{hashExpect, false, false, true, false, bytesExpect}, (*serverRequest_).checkExpect},
		21: {fdesc{hashCacheControl, false, false, false, false, bytesCacheControl}, (*serverRequest_).checkCacheControl},
	}
	serverRequestImportantHeaderFieldFind = func(nameHash uint16) int {
		return (271995178 / int(nameHash)) % len(serverRequestImportantHeaderFieldTable)
	}
)

func (r *serverRequest_) checkAcceptLanguage(subLines []pair, subFrom uint8, subEdge uint8) bool { // Accept-Language = #( language-range [ weight ] )
	// language-range = <language-range, see [RFC4647], Section 2.1>
	// weight = OWS ";" OWS "q=" qvalue
	// qvalue = ( "0" [ "." *3DIGIT ] ) / ( "1" [ "." *3"0" ] )
	if r.zones.acceptLanguage.isEmpty() {
		r.zones.acceptLanguage.from = subFrom
	}
	r.zones.acceptLanguage.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *serverRequest_) checkCacheControl(subLines []pair, subFrom uint8, subEdge uint8) bool { // Cache-Control = #cache-directive
	if r.zCacheControl.isEmpty() {
		r.zCacheControl.from = subFrom
	}
	r.zCacheControl.edge = subEdge
	// cache-directive = token [ "=" ( token / quoted-string ) ]
	for i := subFrom; i < subEdge; i++ {
		// TODO: check for server
	}
	return true
}
func (r *serverRequest_) checkExpect(subLines []pair, subFrom uint8, subEdge uint8) bool { // Expect = #expectation
	// expectation = token [ "=" ( token / quoted-string ) parameters ]
	if r.httpVersion >= Version1_1 {
		if r.zones.expect.isEmpty() {
			r.zones.expect.from = subFrom
		}
		r.zones.expect.edge = subEdge
		for i := subFrom; i < subEdge; i++ {
			subLine := &subLines[i]
			if subLine.kind != pairHeader {
				continue
			}
			subData := subLine.dataAt(r.input)
			bytesToLower(subData) // the Expect field-value is case-insensitive.
			if bytes.Equal(subData, bytes100Continue) {
				r.expectContinue = true
			} else {
				// Unknown expectation, ignored.
			}
		}
	} else { // HTTP/1.0
		// RFC 9110 (section 10.1.1):
		// A server that receives a 100-continue expectation in an HTTP/1.0 request MUST ignore that expectation.
		for i := subFrom; i < subEdge; i++ {
			subLines[i].zero() // since HTTP/1.0 doesn't support 1xx status codes, we delete the expect.
		}
	}
	return true
}
func (r *serverRequest_) checkForwarded(subLines []pair, subFrom uint8, subEdge uint8) bool { // Forwarded = 1#forwarded-element
	if subFrom == subEdge {
		r.headResult, r.failReason = StatusBadRequest, "forwarded = 1#forwarded-element"
		return false
	}
	// forwarded-element = [ forwarded-pair ] *( ";" [ forwarded-pair ] )
	// forwarded-pair    = token "=" value
	// value             = token / quoted-string
	if r.zones.forwarded.isEmpty() {
		r.zones.forwarded.from = subFrom
	}
	r.zones.forwarded.edge = subEdge
	return true
}
func (r *serverRequest_) checkIfMatch(subLines []pair, subFrom uint8, subEdge uint8) bool { // If-Match = "*" / #entity-tag
	return r._checkMatch(subLines, subFrom, subEdge, &r.zones.ifMatch, &r.ifMatch)
}
func (r *serverRequest_) checkIfNoneMatch(subLines []pair, subFrom uint8, subEdge uint8) bool { // If-None-Match = "*" / #entity-tag
	return r._checkMatch(subLines, subFrom, subEdge, &r.zones.ifNoneMatch, &r.ifNoneMatch)
}
func (r *serverRequest_) checkTE(subLines []pair, subFrom uint8, subEdge uint8) bool { // TE = #t-codings
	if r.zones.te.isEmpty() {
		r.zones.te.from = subFrom
	}
	r.zones.te.edge = subEdge
	// t-codings = "trailers" / ( transfer-coding [ t-ranking ] )
	// t-ranking = OWS ";" OWS "q=" rank
	for i := subFrom; i < subEdge; i++ {
		subLine := &subLines[i]
		if subLine.kind != pairHeader {
			continue
		}
		subData := subLine.dataAt(r.input)
		bytesToLower(subData)
		if bytes.Equal(subData, bytesTrailers) {
			r.acceptTrailers = true
		} else if r.httpVersion > Version1_1 {
			r.headResult, r.failReason = StatusBadRequest, "te codings other than trailers are not allowed in http/2 and http/3"
			return false
		}
	}
	return true
}
func (r *serverRequest_) checkUpgrade(subLines []pair, subFrom uint8, subEdge uint8) bool { // Upgrade = #protocol
	if r.httpVersion > Version1_1 {
		r.headResult, r.failReason = StatusBadRequest, "http upgrade is only supported in http/1.1"
		return false
	}
	if r.IsCONNECT() {
		// TODO: confirm this
		return true
	}
	if r.httpVersion == Version1_1 {
		if r.zUpgrade.isEmpty() {
			r.zUpgrade.from = subFrom
		}
		r.zUpgrade.edge = subEdge
		// protocol         = protocol-name ["/" protocol-version]
		// protocol-name    = token
		// protocol-version = token
		for i := subFrom; i < subEdge; i++ {
			subData := subLines[i].dataAt(r.input)
			bytesToLower(subData)
			if bytes.Equal(subData, bytesWebSocket) {
				r.upgradeSocket = true
			} else {
				// Unknown protocol. Ignored. We don't support "Upgrade: h2c" either.
			}
		}
	} else { // HTTP/1.0
		// RFC 9110 (section 7.8):
		// A server that receives an Upgrade header field in an HTTP/1.0 request MUST ignore that Upgrade field.
		for i := subFrom; i < subEdge; i++ {
			subLines[i].zero() // we delete it.
		}
	}
	return true
}
func (r *serverRequest_) checkXForwardedBy(subLines []pair, subFrom uint8, subEdge uint8) bool { // X-Forwarded-By: <proxy1>, <proxy2>
	if subFrom == subEdge {
		r.headResult, r.failReason = StatusBadRequest, "empty x-forwarded-by"
		return false
	}
	if r.zones.xForwardedBy.isEmpty() {
		r.zones.xForwardedBy.from = subFrom
	}
	r.zones.xForwardedBy.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *serverRequest_) checkXForwardedFor(subLines []pair, subFrom uint8, subEdge uint8) bool { // X-Forwarded-For: <client>, <proxy1>, <proxy2>
	if subFrom == subEdge {
		r.headResult, r.failReason = StatusBadRequest, "empty x-forwarded-for"
		return false
	}
	if r.zones.xForwardedFor.isEmpty() {
		r.zones.xForwardedFor.from = subFrom
	}
	r.zones.xForwardedFor.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *serverRequest_) checkXForwardedHost(subLines []pair, subFrom uint8, subEdge uint8) bool { // X-Forwarded-Host = ?
	if subFrom == subEdge {
		r.headResult, r.failReason = StatusBadRequest, "empty x-forwarded-host"
		return false
	}
	if r.zones.xForwardedHost.isEmpty() {
		r.zones.xForwardedHost.from = subFrom
	}
	r.zones.xForwardedHost.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *serverRequest_) checkXForwardedProto(subLines []pair, subFrom uint8, subEdge uint8) bool { // X-Forwarded-Proto = ?
	if subFrom == subEdge {
		r.headResult, r.failReason = StatusBadRequest, "empty x-forwarded-proto"
		return false
	}
	if r.zones.xForwardedProto.isEmpty() {
		r.zones.xForwardedProto.from = subFrom
	}
	r.zones.xForwardedProto.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *serverRequest_) _checkMatch(subLines []pair, subFrom uint8, subEdge uint8, zMatch *zone, match *int8) bool {
	if zMatch.isEmpty() {
		zMatch.from = subFrom
	}
	zMatch.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		subData := subLines[i].dataAt(r.input)
		nMatch := *match // -1:*, 0:nonexist, >0:num
		if len(subData) == 1 && subData[0] == '*' {
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
		r.colonport.set(back, fore)
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
				cookie.nameHash += uint16(b)
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
			cookie.nameHash = 0     // reset for next cookie
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
func (r *serverRequest_) HasRanges() bool      { return r.numRanges > 0 }
func (r *serverRequest_) HasIfRange() bool     { return r.indexes.ifRange != 0 }
func (r *serverRequest_) UserAgent() string    { return string(r.RiskyUserAgent()) }
func (r *serverRequest_) RiskyUserAgent() []byte {
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
func (r *serverRequest_) RiskyCookie(name string) (value []byte, ok bool) {
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
func (r *serverRequest_) AddCookie(name string, value string) bool { // as extra, by webapp
	return r.addExtra(name, value, 0, pairCookie)
}

func (r *serverRequest_) EvalPreconditions(date int64, etag []byte, asOrigin bool) (status int16, normal bool) { // to test against preconditons intentionally
	// Get effective etag without ""
	if n := len(etag); n >= 2 && etag[0] == '"' && etag[n-1] == '"' {
		etag = etag[1 : n-1]
	}
	// RFC 9110 (section 13.2.2):
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
		headerLine := &r.primes[i]
		if headerLine.nameHash != hashIfMatch || !headerLine.nameEqualBytes(r.input, bytesIfMatch) {
			continue
		}
		data := headerLine.dataAt(r.input)
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
		headerLine := &r.primes[i]
		if headerLine.nameHash != hashIfNoneMatch || !headerLine.nameEqualBytes(r.input, bytesIfNoneMatch) {
			continue
		}
		if bytes.Equal(headerLine.valueAt(r.input), etag) {
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
	for i := range r.numRanges {
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
	return r.ranges[:r.numRanges]
}

func (r *serverRequest_) proxyUnsetHost() {
	r._delPrime(r.indexes.host) // zero safe
}
func (r *serverRequest_) proxyDelHopFieldLines(kind int8) {
	delField := r.delHeader
	if kind == pairTrailer {
		delField = r.delTrailer
	}
	if r.zones.te.notEmpty() {
		delField(bytesTE, hashTE)
	}
}
func (r *serverRequest_) proxyWalkHeaderFields(callback func(headerName []byte, headerValue []byte) bool) bool { // TODO: closure escapes to heap?
	// TODO
	// RFC 3875 (section 4.1.18): If multiple header fields with the same field-name are received then the server MUST rewrite them as a single value having the same semantics.
	// Note: check headerLine.isUnderscore()
	// Note: // TODO: got a "foo_bar" header line and user prefer it. avoid name conflicts with header line which is like "foo-bar"
	return true
}

func (r *serverRequest_) proxyWalkCookies(callback func(cookie *pair, cookieName []byte, cookieValue []byte) bool) bool { // TODO: closure escapes to heap?
	for i := r.cookies.from; i < r.cookies.edge; i++ {
		if cookie := &r.primes[i]; cookie.nameHash != 0 {
			if !callback(cookie, cookie.nameAt(r.input), cookie.valueAt(r.input)) {
				return false
			}
		}
	}
	if r.hasExtra[pairCookie] {
		for i := range len(r.extras) {
			if extra := &r.extras[i]; extra.nameHash != 0 && extra.kind == pairCookie {
				if !callback(extra, extra.nameAt(r.array), extra.valueAt(r.array)) {
					return false
				}
			}
		}
	}
	return true
}

func (r *serverRequest_) HasContent() bool { return r.contentSize >= 0 || r.IsVague() }
func (r *serverRequest_) Content() string  { return string(r.RiskyContent()) }
func (r *serverRequest_) RiskyContent() []byte {
	if r.formKind == httpFormMultipart { // loading multipart form into memory is not allowed!
		return nil
	}
	return r.riskyContent()
}

func (r *serverRequest_) contentIsForm() bool { return r.formKind != httpFormNotForm }
func (r *serverRequest_) parseHTMLForm() { // called on need to populate r.forms and r.upfiles
	if !r.contentIsForm() || r.formReceived {
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
		state = 2 // to be consistent with HTTP/1.x
		octet byte
	)
	form := &r.mainPair
	form.zero()
	form.kind = pairForm
	form.place = placeArray // all received forms are placed in r.array
	form.nameFrom = r.arrayEdge
	for i := range r.receivedSize { // TODO: use a better algorithm to improve performance
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
				form.nameHash += uint16(b)
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
				form.nameHash = 0 // reset for next form
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
					form.nameHash += uint16(octet)
				}
				r.arrayPush(octet)
				state >>= 4 // restore last state
			}
		}
	}
	// Reaches the end of content.
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
	r.elemBack, r.elemFore = 0, 0
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
		case tempFile: // [0, r.webapp.maxMultiformSize]. case happens when sized content > 64K1, or content is vague.
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
			defer func() { // TODO: closure escapes to heap?
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
	template := r.RiskyMake(3 + r.boundary.size() + 2) // \n--boundary--
	template[0], template[1], template[2] = '\n', '-', '-'
	n := 3 + copy(template[3:], r.input[r.boundary.from:r.boundary.edge])
	separator := template[0:n] // \n--boundary
	template[n], template[n+1] = '-', '-'
	for { // each part in multipart
		// Now r.formWindow is used for receiving --boundary-- EOL or --boundary EOL
		for r.formWindow[r.elemFore] != '\n' {
			if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
				return
			}
		}
		if r.elemBack == r.elemFore {
			r.stream.markBroken()
			return
		}
		fore := r.elemFore
		if fore >= 1 && r.formWindow[fore-1] == '\r' {
			fore--
		}
		if bytes.Equal(r.formWindow[r.elemBack:fore], template[1:n+2]) { // end of multipart (--boundary--)
			// All parts are received.
			if DebugLevel() >= 2 {
				Println(r.arrayEdge, cap(r.array), string(r.array[0:r.arrayEdge]))
			}
			return
		} else if !bytes.Equal(r.formWindow[r.elemBack:fore], template[1:n]) { // not start of multipart (--boundary)
			r.stream.markBroken()
			return
		}
		// Skip '\n'
		if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
			return
		}
		// r.elemFore is at fields of current part.
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
			if b := r.formWindow[r.elemFore]; b == '\r' {
				if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
					return
				}
				if r.formWindow[r.elemFore] != '\n' {
					r.stream.markBroken()
					return
				}
				break
			} else if b == '\n' {
				break
			}
			r.elemBack = r.elemFore // now r.formWindow is used for receiving field-name and onward
			for {                   // field name
				b := r.formWindow[r.elemFore]
				if t := httpTchar[b]; t == 1 {
					// Fast path, do nothing
				} else if t == 2 { // A-Z
					r.formWindow[r.elemFore] = b + 0x20 // to lower
				} else if t == 3 { // '_'
					// For forms, do nothing
				} else if b == ':' {
					break
				} else {
					r.stream.markBroken()
					return
				}
				if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
					return
				}
			}
			if r.elemBack == r.elemFore { // field-name cannot be empty
				r.stream.markBroken()
				return
			}
			r.pFieldName.set(r.elemBack, r.elemFore) // in case of sliding r.formWindow when r._growMultipartForm()
			// Skip ':'
			if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
				return
			}
			// Skip OWS before field value
			for r.formWindow[r.elemFore] == ' ' || r.formWindow[r.elemFore] == '\t' {
				if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
					return
				}
			}
			r.elemBack = r.elemFore
			// Now r.formWindow is used for receiving field-value and onward. at this time we can still use r.pFieldName, no risk of sliding
			if fieldName := r.formWindow[r.pFieldName.from:r.pFieldName.edge]; bytes.Equal(fieldName, bytesContentDisposition) { // content-disposition
				// form-data; name="avatar"; filename="michael.jpg"
				for r.formWindow[r.elemFore] != ';' {
					if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
				if r.elemBack == r.elemFore || !bytes.Equal(r.formWindow[r.elemBack:r.elemFore], bytesFormData) {
					r.stream.markBroken()
					return
				}
				r.elemBack = r.elemFore // now r.formWindow is used for receiving parameters and onward
				for r.formWindow[r.elemFore] != '\n' {
					if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
				fore := r.elemFore
				if r.formWindow[fore-1] == '\r' {
					fore--
				}
				// Skip OWS after field value
				for r.formWindow[fore-1] == ' ' || r.formWindow[fore-1] == '\t' {
					fore--
				}
				paras := make([]para, 2) // for name & filename. won't escape to heap
				n, ok := r._parseParas(r.formWindow, r.elemBack, fore, paras)
				if !ok {
					r.stream.markBroken()
					return
				}
				for i := range n { // each para in field (; name="avatar"; filename="michael.jpg")
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
						nameBuffer := r.stream.buffer256() // enough for temp name
						m := r.stream.MakeTempName(nameBuffer, time.Now().Unix())
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
				for r.formWindow[r.elemFore] != '\n' {
					if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
				fore := r.elemFore
				if r.formWindow[fore-1] == '\r' {
					fore--
				}
				// Skip OWS after field value
				for r.formWindow[fore-1] == ' ' || r.formWindow[fore-1] == '\t' {
					fore--
				}
				if n := fore - r.elemBack; n == 0 || n > 255 {
					r.stream.markBroken()
					return
				}
				part.type_.from = r.arrayEdge
				if !r.arrayCopy(r.formWindow[r.elemBack:fore]) { // add "image/jpeg"
					r.stream.markBroken()
					return
				}
				part.type_.edge = r.arrayEdge
			} else { // other fields are ignored
				for r.formWindow[r.elemFore] != '\n' {
					if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
						return
					}
				}
			}
			// Skip '\n' and goto next field or end of fields
			if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
				return
			}
		}
		if !part.valid { // no valid fields
			r.stream.markBroken()
			return
		}
		// Now all fields of the part are received. Skip end of fields and goto part data
		if r.elemFore++; r.elemFore == r.formEdge && !r._growMultipartForm() {
			return
		}
		if part.isFile {
			// TODO: upload code
			part.upfile.nameHash = part.hash
			part.upfile.nameSize, part.upfile.nameFrom = uint8(part.name.size()), part.name.from
			part.upfile.baseSize, part.upfile.baseFrom = uint8(part.base.size()), part.base.from
			part.upfile.typeSize, part.upfile.typeFrom = uint8(part.type_.size()), part.type_.from
			part.upfile.pathSize, part.upfile.pathFrom = uint8(part.path.size()), part.path.from
			if osFile, err := os.OpenFile(string(r.array[part.path.from:part.path.edge]), os.O_RDWR|os.O_CREATE, 0644); err == nil {
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
			part.form.nameHash = part.hash
			part.form.nameFrom = part.name.from
			part.form.nameSize = uint8(part.name.size())
			part.form.value.from = r.arrayEdge
		}
		r.elemBack = r.elemFore // now r.formWindow is used for receiving part data and onward
		for {                   // each partial in current part
			partial := r.formWindow[r.elemBack:r.formEdge]
			r.elemFore = r.formEdge
			mode := 0 // by default, we assume end of part ("\n--boundary") is not in partial
			var i int
			if i = bytes.Index(partial, separator); i >= 0 {
				mode = 1 // end of part ("\n--boundary") is found in partial
			} else if i = bytes.LastIndexByte(partial, '\n'); i >= 0 && bytes.HasPrefix(separator, partial[i:]) {
				mode = 2 // partial ends with prefix of end of part ("\n--boundary")
			}
			if mode > 0 { // found "\n" at i
				r.elemFore = r.elemBack + int32(i)
				if r.elemFore > r.elemBack && r.formWindow[r.elemFore-1] == '\r' {
					r.elemFore--
				}
				partial = r.formWindow[r.elemBack:r.elemFore] // pure data
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
				r.elemBack += int32(i + 1) // at the first '-' of "--boundary"
				r.elemFore = r.elemBack    // next part starts here
				break                      // part is received.
			}
			if mode == 2 {
				r.elemBack = r.elemFore // from EOL (\r or \n). need more and continue
			} else { // mode == 0
				r.elemBack, r.formEdge = 0, 0 // pure data, clean r.formWindow. need more and continue
			}
			// Grow more
			if !r._growMultipartForm() {
				return
			}
		}
	}
}
func (r *serverRequest_) _growMultipartForm() bool { // caller needs more data from content file
	if r.consumedSize == r.receivedSize || (r.formEdge == int32(len(r.formWindow)) && r.elemBack == 0) {
		r.stream.markBroken()
		return false
	}
	if r.elemBack > 0 { // have useless data. slide to start
		copy(r.formWindow, r.formWindow[r.elemBack:r.formEdge])
		r.formEdge -= r.elemBack
		r.elemFore -= r.elemBack
		if r.pFieldName.notEmpty() {
			r.pFieldName.sub(r.elemBack) // for fields in multipart/form-data, not for trailer fields
		}
		r.elemBack = 0
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
func (r *serverRequest_) RiskyForm(name string) (value []byte, ok bool) {
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
func (r *serverRequest_) AddForm(name string, value string) bool { // as extra, by webapp
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
	for i := range len(r.upfiles) {
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
		nameHash := stringHash(name)
		for i := range n {
			if upfile := &r.upfiles[i]; upfile.nameHash == nameHash && upfile.nameEqualString(r.array, name) {
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
		nameHash := stringHash(name)
		for i := range n {
			if upfile := &r.upfiles[i]; upfile.nameHash == nameHash && upfile.nameEqualString(r.array, name) {
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
	for i := r.trailerLines.from; i < r.trailerLines.edge; i++ {
		if !r.applyTrailerLine(i) {
			// r.bodyResult is set.
			return false
		}
	}
	return true
}
func (r *serverRequest_) applyTrailerLine(lineIndex uint8) bool {
	//trailerLine := &r.primes[lineIndex]
	// TODO: Pseudo-header fields MUST NOT appear in a trailer section.
	return true
}

func (r *serverRequest_) hookReviser(reviser Reviser) { // to revise input content
	r.hasRevisers = true
	r.revisers[reviser.Rank()] = reviser.ID() // revisers are placed to fixed position, by their ranks.
}

func (r *serverRequest_) riskyVariable(varCode int16, varName string) (varValue []byte) {
	if varCode != -1 {
		return serverRequestVariables[varCode](r)
	}
	if strings.HasPrefix(varName, "header_") {
		varName = varName[len("header_"):]
		if v, ok := r.RiskyHeader(varName); ok {
			return v
		}
	} else if strings.HasPrefix(varName, "cookie_") {
		varName = varName[len("cookie_"):]
		if v, ok := r.RiskyCookie(varName); ok {
			return v
		}
	} else if strings.HasPrefix(varName, "query_") {
		varName = varName[len("query_"):]
		if v, ok := r.RiskyQuery(varName); ok {
			return v
		}
	}
	return nil
}

var serverRequestVariables = [...]func(*serverRequest_) []byte{ // keep sync with varCodes
	0: (*serverRequest_).RiskyMethod,      // method
	1: (*serverRequest_).RiskyScheme,      // scheme
	2: (*serverRequest_).RiskyAuthority,   // authority
	3: (*serverRequest_).RiskyHostname,    // hostname
	4: (*serverRequest_).RiskyColonport,   // colonport
	5: (*serverRequest_).RiskyPath,        // path
	6: (*serverRequest_).RiskyURI,         // uri
	7: (*serverRequest_).RiskyEncodedPath, // encodedPath
	8: (*serverRequest_).RiskyQueryString, // queryString
	9: (*serverRequest_).RiskyContentType, // contentType
}

// ServerResponse is the server-side http response.
type ServerResponse interface { // for *server[1-3]Response
	Request() ServerRequest

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
	SendBadRequest(content []byte) error                                                 // 400
	SendForbidden(content []byte) error                                                  // 403
	SendNotFound(content []byte) error                                                   // 404
	SendMethodNotAllowed(allow string, content []byte) error                             // 405
	SendNotAcceptable(content []byte) error                                              // 406
	SendUnsupportedMediaType(acceptEncoding string, accept string, content []byte) error // 415
	SendRangeNotSatisfiable(contentSize int64, content []byte) error                     // 416
	SendInternalServerError(content []byte) error                                        // 500
	SendNotImplemented(content []byte) error                                             // 501
	SendBadGateway(content []byte) error                                                 // 502
	SendGatewayTimeout(content []byte) error                                             // 504

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
	pickOutRanges(ranges []Range, rangeType string)
	sendText(content []byte) error
	sendFile(content *os.File, info os.FileInfo, shut bool) error // will close content after sent
	sendChain() error                                             // content
	echoHeaders() error
	echoChain() error // chunks
	addTrailer(name []byte, value []byte) bool
	endVague() error
	proxyPass1xx(backResp BackendResponse) bool
	proxyPassMessage(backResp BackendResponse) error              // pass content to client directly
	proxyPostMessage(backContent any, backHasTrailers bool) error // post held content to client
	proxyCopyHeaderLines(backResp BackendResponse, proxyConfig *HTTPProxyConfig) bool
	proxyCopyTrailerLines(backResp BackendResponse, proxyConfig *HTTPProxyConfig) bool
	hookReviser(reviser Reviser)
	riskyMake(size int) []byte
}

// serverResponse_ is a parent.
type serverResponse_ struct { // for server[1-3]Response. outgoing, needs building
	// Mixins
	_httpOut_ // outgoing http response
	// Assocs
	request ServerRequest // related request
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	status    int16    // 200, 302, 404, 500, ...
	_         [6]byte  // padding
	start     [16]byte // exactly 16 bytes for "HTTP/1.1 NNN X\r\n". also used by HTTP/2 and HTTP/3, but shorter
	unixTimes struct { // in seconds
		expires      int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
		lastModified int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
	}
	// Stream states (zeros)
	webapp           *Webapp // associated webapp
	_serverResponse0         // all values in this struct must be zero by default!
}
type _serverResponse0 struct { // for fast reset, entirely
	indexes struct {
		expires      uint8
		lastModified uint8
		_            [6]byte // padding
	}
	revisers [32]uint8 // reviser ids which will apply on this response. indexed by reviser order
}

func (r *serverResponse_) onUse(httpVersion uint8) { // for non-zeros
	r._httpOut_.onUse(httpVersion, false) // as response

	r.status = StatusOK
	r.unixTimes.expires = -1      // not set
	r.unixTimes.lastModified = -1 // not set
}
func (r *serverResponse_) onEnd() { // for zeros
	r.webapp = nil
	r._serverResponse0 = _serverResponse0{}

	r._httpOut_.onEnd()
}

func (r *serverResponse_) Request() ServerRequest { return r.request }

func (r *serverResponse_) SetStatus(status int16) error {
	if status >= 200 && status <= 999 {
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
func (r *serverResponse_) controlData() []byte { // used by http/2 and http/3. http/1.x overrides this!
	start := r.start[:len(httpStatus)]
	if r.status < int16(len(http1Controls)) && http1Controls[r.status] != nil {
		control := http1Controls[r.status]
		start[8] = control[9]
		start[9] = control[10]
		start[10] = control[11]
	} else {
		copy(start, httpStatus[:])
		start[8] = byte(r.status/100 + '0')
		start[9] = byte(r.status/10%10 + '0')
		start[10] = byte(r.status%10 + '0')
	}
	return start
}

func (r *serverResponse_) MakeETagFrom(date int64, size int64) ([]byte, bool) { // with ""
	if date < 0 || size < 0 {
		return nil, false
	}
	p := r.riskyMake(32)
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
func (r *serverResponse_) SendNotAcceptable(content []byte) error { // 406
	return r.sendError(StatusNotAcceptable, content)
}
func (r *serverResponse_) SendUnsupportedMediaType(acceptEncoding string, accept string, content []byte) error { // 415
	if acceptEncoding != "" {
		r.AddHeaderBytes(bytesAcceptEncoding, ConstBytes(acceptEncoding))
	}
	if accept != "" {
		r.AddHeaderBytes(bytesAccept, ConstBytes(accept))
	}
	return r.sendError(StatusUnsupportedMediaType, content)
}
func (r *serverResponse_) SendRangeNotSatisfiable(contentSize int64, content []byte) error { // 416
	// add a header like: content-range: bytes */1234
	valueBuffer := r.stream.buffer256() // enough for content range
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
	return r.out.sendChain()
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
		phrase := control[len("HTTP/1.1 NNN ") : len(control)-2]
		pages[int16(status)] = []byte(fmt.Sprintf(template, status, phrase, status, phrase))
	}
	return pages
}()

func (r *serverResponse_) beforeSend() {
	servResp := r.out.(ServerResponse)
	for _, id := range r.revisers {
		if id == 0 { // id of effective reviser is ensured to be > 0
			continue
		}
		reviser := r.webapp.reviserByID(id)
		reviser.BeforeSend(servResp.Request(), servResp) // revise header fields
	}
}
func (r *serverResponse_) doSend() error {
	if r.hasRevisers {
		servResp := r.out.(ServerResponse)
		for _, id := range r.revisers { // revise sized content
			if id == 0 {
				continue
			}
			reviser := r.webapp.reviserByID(id)
			reviser.OnOutput(servResp.Request(), servResp, &r.chain)
		}
		// Because r.chain may be altered by revisers, content size must be recalculated
		if contentSize, ok := r.chain.Size(); ok {
			r.contentSize = contentSize
		} else {
			return httpOutTooLarge
		}
	}
	return r.out.sendChain()
}

func (r *serverResponse_) beforeEcho() {
	servResp := r.out.(ServerResponse)
	for _, id := range r.revisers { // revise header fields
		if id == 0 { // id of effective reviser is ensured to be > 0
			continue
		}
		reviser := r.webapp.reviserByID(id)
		reviser.BeforeEcho(servResp.Request(), servResp)
	}
}
func (r *serverResponse_) doEcho() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	r.chain.PushTail(&r.piece)
	defer r.chain.free()
	if r.hasRevisers {
		servResp := r.out.(ServerResponse)
		for _, id := range r.revisers { // revise vague content
			if id == 0 { // id of effective reviser is ensured to be > 0
				continue
			}
			reviser := r.webapp.reviserByID(id)
			reviser.OnOutput(servResp.Request(), servResp, &r.chain)
		}
	}
	return r.out.echoChain()
}
func (r *serverResponse_) endVague() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	if r.hasRevisers {
		servResp := r.out.(ServerResponse)
		for _, id := range r.revisers { // finish vague content
			if id == 0 { // id of effective reviser is ensured to be > 0
				continue
			}
			reviser := r.webapp.reviserByID(id)
			reviser.FinishEcho(servResp.Request(), servResp)
		}
	}
	return r.out.finalizeVague()
}

var ( // minimal perfect hash table for response critical header fields
	serverResponseCriticalHeaderFieldTable = [10]struct {
		hash uint16
		name []byte
		fAdd func(*serverResponse_, []byte) (ok bool)
		fDel func(*serverResponse_) (deleted bool)
	}{ // connection content-length content-type date expires last-modified server set-cookie transfer-encoding upgrade
		0: {hashServer, bytesServer, nil, nil},       // restricted. added at finalizeHeaders()
		1: {hashSetCookie, bytesSetCookie, nil, nil}, // restricted. use specific api to add
		2: {hashUpgrade, bytesUpgrade, nil, nil},     // restricted. not allowed to change the protocol. may be added if webSocket?
		3: {hashDate, bytesDate, (*serverResponse_)._insertDate, (*serverResponse_)._removeDate},
		4: {hashTransferEncoding, bytesTransferEncoding, nil, nil}, // restricted. added at finalizeHeaders() if needed
		5: {hashConnection, bytesConnection, nil, nil},             // restricted. added at finalizeHeaders()
		6: {hashLastModified, bytesLastModified, (*serverResponse_)._insertLastModified, (*serverResponse_)._removeLastModified},
		7: {hashExpires, bytesExpires, (*serverResponse_)._insertExpires, (*serverResponse_)._removeExpires},
		8: {hashContentLength, bytesContentLength, nil, nil}, // restricted. added at finalizeHeaders()
		9: {hashContentType, bytesContentType, (*serverResponse_)._insertContentType, (*serverResponse_)._removeContentType},
	}
	serverResponseCriticalHeaderFieldFind = func(nameHash uint16) int {
		return (113100 / int(nameHash)) % len(serverResponseCriticalHeaderFieldTable)
	}
)

func (r *serverResponse_) insertHeader(nameHash uint16, name []byte, value []byte) bool {
	h := &serverResponseCriticalHeaderFieldTable[serverResponseCriticalHeaderFieldFind(nameHash)]
	if h.hash == nameHash && bytes.Equal(h.name, name) {
		if h.fAdd == nil { // mainly because this header field is restricted to insert
			return true // pretend to be successful
		}
		return h.fAdd(r, value)
	}
	return r.out.addHeader(name, value)
}
func (r *serverResponse_) _insertExpires(expires []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.expires, &r.indexes.expires, bytesExpires, expires)
}
func (r *serverResponse_) _insertLastModified(lastModified []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.lastModified, &r.indexes.lastModified, bytesLastModified, lastModified)
}

func (r *serverResponse_) removeHeader(nameHash uint16, name []byte) bool {
	h := &serverResponseCriticalHeaderFieldTable[serverResponseCriticalHeaderFieldFind(nameHash)]
	if h.hash == nameHash && bytes.Equal(h.name, name) {
		if h.fDel == nil { // mainly because this header field is restricted to remove
			return true // pretend to be successful
		}
		return h.fDel(r)
	}
	return r.out.delHeader(name)
}
func (r *serverResponse_) _removeExpires() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.expires, &r.indexes.expires)
}
func (r *serverResponse_) _removeLastModified() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.lastModified, &r.indexes.lastModified)
}

func (r *serverResponse_) proxyPassMessage(backResp BackendResponse) error {
	return r._proxyPassMessage(backResp)
}
func (r *serverResponse_) proxyCopyHeaderLines(backResp BackendResponse, proxyConfig *HTTPProxyConfig) bool {
	backResp.proxyDelHopHeaderFields()

	// Copy control (:status)
	r.SetStatus(backResp.Status())

	// Copy selective forbidden header fields (excluding set-cookie, which is copied directly) from backResp

	// Copy added header fields
	for headerName, vHeaderValue := range proxyConfig.AddResponseHeaders {
		var headerValue []byte
		if vHeaderValue.IsVariable() {
			headerValue = vHeaderValue.BytesVar(r.Request())
		} else if v, ok := vHeaderValue.Bytes(); ok {
			headerValue = v
		} else {
			// Invalid values are treated as empty
		}
		if !r.out.addHeader(ConstBytes(headerName), headerValue) {
			return false
		}
	}

	// Copy remaining header fields from backResp
	if !backResp.proxyWalkHeaderLines(r.out, func(out httpOut, headerLine *pair, headerName []byte, lineValue []byte) bool {
		if headerLine.nameHash == hashSetCookie && bytes.Equal(headerName, bytesSetCookie) { // set-cookie is copied directly
			return out.addHeader(headerName, lineValue)
		} else {
			return out.insertHeader(headerLine.nameHash, headerName, lineValue) // some header fields (e.g. "connection") are restricted
		}
	}) {
		return false
	}

	// This must be placed at the end so we can delete some header fields forcely.
	for _, headerName := range proxyConfig.DelResponseHeaders {
		r.out.delHeader(headerName)
	}

	return true
}
func (r *serverResponse_) proxyCopyTrailerLines(backResp BackendResponse, proxyConfig *HTTPProxyConfig) bool {
	return backResp.proxyWalkTrailerLines(r.out, func(out httpOut, trailerLine *pair, trailerName []byte, lineValue []byte) bool {
		return out.addTrailer(trailerName, lineValue)
	})
}

func (r *serverResponse_) hookReviser(reviser Reviser) { // to revise output content
	r.hasRevisers = true
	r.revisers[reviser.Rank()] = reviser.ID() // revisers are placed to fixed position, by their ranks.
}

// ServerSocket is the server-side webSocket.
type ServerSocket interface { // for *server[1-3]Socket
	// TODO
	Read(dst []byte) (int, error)
	Write(src []byte) (int, error)
	Close() error
}

// serverSocket_ is a parent.
type serverSocket_ struct { // for server[1-3]Socket. incoming and outgoing
	// Mixins
	_httpSocket_
	// Assocs
	// Stream states (non-zeros)
	// Stream states (zeros)
	_serverSocket0 // all values in this struct must be zero by default!
}
type _serverSocket0 struct { // for fast reset, entirely
}

func (s *serverSocket_) onUse() {
	const asServer = true
	s._httpSocket_.onUse(asServer)
}
func (s *serverSocket_) onEnd() {
	s._serverSocket0 = _serverSocket0{}

	s._httpSocket_.onEnd()
}

func (s *serverSocket_) serverTodo() {
}
