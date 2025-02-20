// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP backend implementation. See RFC 9110.

package hemi

import (
	"bytes"
	"errors"
	"time"
)

// HTTPBackend is the http backend.
type HTTPBackend interface { // for *HTTP[1-3]Backend
	// Imports
	Backend
	// Methods
	AcquireStream(servReq ServerRequest) (BackendStream, error)
	ReleaseStream(backStream BackendStream)
}

// httpBackend_
type httpBackend_[N HTTPNode] struct { // for HTTP[1-3]Backend
	// Parent
	Backend_[N]
	// States
}

func (b *httpBackend_[N]) OnConfigure() {
	b.Backend_.OnConfigure()
	b.ConfigureNodes()
}
func (b *httpBackend_[N]) OnPrepare() {
	b.Backend_.OnPrepare()
	b.PrepareNodes()
}

// HTTPNode is the http node.
type HTTPNode interface {
	// Imports
	Node
	httpHolder
	// Methods
}

// httpNode_ is a parent.
type httpNode_[B HTTPBackend] struct { // for http[1-3]Node
	// Parent
	Node_[B]
	// Mixins
	_httpHolder_ // holds conns
	// States
	keepAliveConns int32         // max conns to keep alive
	idleTimeout    time.Duration // conn idle timeout
}

func (n *httpNode_[B]) onCreate(compName string, stage *Stage, backend B) {
	n.Node_.OnCreate(compName, stage, backend)
}

func (n *httpNode_[B]) onConfigure() {
	n.Node_.OnConfigure()
	n._httpHolder_.onConfigure(n, 0*time.Second, 0*time.Second, TmpDir()+"/web/backends/"+n.backend.CompName()+"/"+n.compName)

	// .keepAliveConns
	n.ConfigureInt32("keepAliveConns", &n.keepAliveConns, func(value int32) error {
		if value > 0 {
			return nil
		}
		return errors.New("bad keepAliveConns in node")
	}, 10)

	// .idleTimeout
	n.ConfigureDuration("idleTimeout", &n.idleTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".idleTimeout has an invalid value")
	}, 2*time.Second)
}
func (n *httpNode_[B]) onPrepare() {
	n.Node_.OnPrepare()
	n._httpHolder_.onPrepare(n, 0755)
}

// backendConn is the backend-side http connection.
type backendConn interface {
}

// _backendConn_ is a mixin.
type _backendConn_[N HTTPNode] struct { // for backend[1-3]Conn
	// Conn states (stocks)
	// Conn states (controlled)
	expireTime time.Time // when the conn is considered expired
	// Conn states (non-zeros)
	node N // the node to which the connection belongs
	// Conn states (zeros)
}

func (c *_backendConn_[N]) onGet(node N) {
	c.node = node
}
func (c *_backendConn_[N]) onPut() {
	c.expireTime = time.Time{}
	var null N // nil
	c.node = null
}

func (c *_backendConn_[N]) Holder() httpHolder { return c.node }

func (c *_backendConn_[N]) isAlive() bool {
	return c.expireTime.IsZero() || time.Now().Before(c.expireTime)
}

// BackendStream is the backend-side http stream.
type BackendStream interface { // for *backend[1-3]Stream
	Response() BackendResponse
	Request() BackendRequest
	Socket() BackendSocket

	isBroken() bool
	markBroken()
}

// _backendStream_ is a mixin.
type _backendStream_ struct { // for backend[1-3]Stream
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *_backendStream_) onUse() {
}
func (s *_backendStream_) onEnd() {
}

// BackendResponse is the backend-side http response.
type BackendResponse interface { // for *backend[1-3]Response
	KeepAlive() bool
	HeadResult() int16
	BodyResult() int16
	Status() int16
	HasContent() bool
	ContentSize() int64
	HasTrailers() bool
	IsVague() bool

	// Internal only
	recvHead()
	reuse()
	examineTail() bool
	readContent() (data []byte, err error)
	proxyTakeContent() any
	proxyDelHopHeaderFields()
	proxyDelHopTrailerFields()
	proxyDelHopFieldLines(kind int8)
	proxyWalkHeaderLines(out httpOut, callback func(out httpOut, headerLine *pair, headerName []byte, lineValue []byte) bool) bool
	proxyWalkTrailerLines(out httpOut, callback func(out httpOut, trailerLine *pair, trailerName []byte, lineValue []byte) bool) bool
}

// backendResponse_ is a parent.
type backendResponse_ struct { // for backend[1-3]Response. incoming, needs parsing
	// Mixins
	_httpIn_ // incoming http response
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	_backendResponse0 // all values in this struct must be zero by default!
}
type _backendResponse0 struct { // for fast reset, entirely
	status      int16    // 200, 302, 404, ...
	acceptBytes bool     // accept-ranges: bytes?
	hasAllow    bool     // has "allow" header field?
	ageSeconds  int32    // age in seconds
	indexes     struct { // indexes of some selected singleton header fields, for fast accessing
		age                uint8 // age header line ->r.input
		contentDisposition uint8 // content-disposition header line ->r.input
		etag               uint8 // etag header line ->r.input
		expires            uint8 // expires header line ->r.input
		lastModified       uint8 // last-modified header line ->r.input
		location           uint8 // location header line ->r.input
		retryAfter         uint8 // retry-after header line ->r.input
		server             uint8 // server header line ->r.input
	}
	zones struct { // zones (may not be continuous) of some selected important header fields, for fast accessing
		acceptRanges      zone
		allow             zone
		altSvc            zone
		cacheStatus       zone
		cdnCacheControl   zone
		proxyAuthenticate zone
		vary              zone
		wwwAuthenticate   zone
	}
	unixTimes struct { // parsed unix times in seconds
		expires      int64 // parsed unix time of expires
		lastModified int64 // parsed unix time of last-modified
	}
	cacheControl struct { // the cache-control info
		noCache         bool  // no-cache directive in cache-control
		noStore         bool  // no-store directive in cache-control
		noTransform     bool  // no-transform directive in cache-control
		public          bool  // public directive in cache-control
		private         bool  // private directive in cache-control
		mustRevalidate  bool  // must-revalidate directive in cache-control
		mustUnderstand  bool  // must-understand directive in cache-control
		proxyRevalidate bool  // proxy-revalidate directive in cache-control
		maxAge          int32 // max-age directive in cache-control
		sMaxage         int32 // s-maxage directive in cache-control
	}
}

func (r *backendResponse_) onUse(httpVersion uint8) { // for non-zeros
	r._httpIn_.onUse(httpVersion, true) // as response
}
func (r *backendResponse_) onEnd() { // for zeros
	r._backendResponse0 = _backendResponse0{}

	r._httpIn_.onEnd()
}

func (r *backendResponse_) reuse() { // between 1xx and non-1xx responses
	httpVersion := r.httpVersion
	r.onEnd() // this clears r.httpVersion
	r.onUse(httpVersion)
}

func (r *backendResponse_) Status() int16 { return r.status }

func (r *backendResponse_) examineHead() bool {
	for i := r.headerLines.from; i < r.headerLines.edge; i++ {
		if !r._applyHeaderLine(i) {
			// r.headResult is set.
			return false
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

	// Basic checks against versions
	switch r.httpVersion {
	case Version1_0: // we don't support HTTP/1.0 in backend side!
		BugExitln("HTTP/1.0 must be denied priorly")
	case Version1_1:
		if r.keepAlive == -1 { // no connection header field
			r.keepAlive = 1 // default is keep-alive for HTTP/1.1
		}
	default: // HTTP/2 and HTTP/3
		r.keepAlive = 1 // default is keep-alive for HTTP/2 and HTTP/3
		// TODO: add checks here
	}

	if !r.determineContentMode() {
		// r.headResult is set.
		return false
	}
	if r.contentSize != -1 && (r.status < StatusOK || r.status == StatusNoContent) { // TODO: what about 304?
		r.headResult, r.failReason = StatusBadRequest, "content is not allowed in 1xx and 204 responses"
		return false
	}
	if r.contentSize > r.maxContentSize {
		r.headResult, r.failReason = StatusContentTooLarge, "content size exceeds backend's limit"
		return false
	}

	return true
}
func (r *backendResponse_) _applyHeaderLine(lineIndex uint8) bool {
	headerLine := &r.primes[lineIndex]
	headerName := headerLine.nameAt(r.input)
	if sh := &backendResponseSingletonHeaderFieldTable[backendResponseSingletonHeaderFieldFind(headerLine.nameHash)]; sh.nameHash == headerLine.nameHash && bytes.Equal(sh.name, headerName) {
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
	} else if mh := &backendResponseImportantHeaderFieldTable[backendResponseImportantHeaderFieldFind(headerLine.nameHash)]; mh.nameHash == headerLine.nameHash && bytes.Equal(mh.name, headerName) {
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

var ( // minimal perfect hash table for singleton response header fields
	backendResponseSingletonHeaderFieldTable = [14]struct {
		parse bool // need general parse or not
		fdesc      // allowQuote, allowEmpty, allowParam, hasComment
		check func(*backendResponse_, *pair, uint8) bool
	}{ // age content-disposition content-length content-location content-range content-type date etag expires last-modified location retry-after server set-cookie
		0:  {false, fdesc{hashLastModified, false, false, false, false, bytesLastModified}, (*backendResponse_).checkLastModified},
		1:  {true, fdesc{hashContentLocation, true, false, false, false, bytesContentLocation}, (*backendResponse_).checkContentLocation},
		2:  {false, fdesc{hashSetCookie, false, false, false, false, bytesSetCookie}, (*backendResponse_).checkSetCookie}, // `a=b; Path=/; HttpsOnly` is not parameters
		3:  {false, fdesc{hashContentRange, false, false, false, false, bytesContentRange}, (*backendResponse_).checkContentRange},
		4:  {false, fdesc{hashETag, false, false, false, false, bytesETag}, (*backendResponse_).checkETag},
		5:  {false, fdesc{hashRetryAfter, false, false, false, false, bytesRetryAfter}, (*backendResponse_).checkRetryAfter},
		6:  {false, fdesc{hashLocation, false, false, false, false, bytesLocation}, (*backendResponse_).checkLocation},
		7:  {false, fdesc{hashServer, false, false, false, true, bytesServer}, (*backendResponse_).checkServer},
		8:  {false, fdesc{hashContentDisposition, true, false, true, false, bytesContentDisposition}, (*backendResponse_).checkContentDisposition},
		9:  {true, fdesc{hashContentType, false, false, true, false, bytesContentType}, (*backendResponse_).checkContentType},
		10: {false, fdesc{hashDate, false, false, false, false, bytesDate}, (*backendResponse_).checkDate},
		11: {false, fdesc{hashContentLength, false, false, false, false, bytesContentLength}, (*backendResponse_).checkContentLength},
		12: {false, fdesc{hashAge, false, false, false, false, bytesAge}, (*backendResponse_).checkAge},
		13: {false, fdesc{hashExpires, false, false, false, false, bytesExpires}, (*backendResponse_).checkExpires},
	}
	backendResponseSingletonHeaderFieldFind = func(nameHash uint16) int {
		return (3568946 / int(nameHash)) % len(backendResponseSingletonHeaderFieldTable)
	}
)

func (r *backendResponse_) checkAge(headerLine *pair, lineIndex uint8) bool { // Age = delta-seconds
	if headerLine.value.isEmpty() {
		r.headResult, r.failReason = StatusBadRequest, "empty age"
		return false
	}
	// TODO: check and write to r.ageSeconds
	r.indexes.age = lineIndex
	return true
}
func (r *backendResponse_) checkContentDisposition(headerLine *pair, lineIndex uint8) bool { // Content-Disposition = disposition-type *( ";" disposition-parm )
	// TODO: check
	r.indexes.contentDisposition = lineIndex
	return true
}
func (r *backendResponse_) checkETag(headerLine *pair, lineIndex uint8) bool { // ETag = entity-tag
	// TODO: check
	r.indexes.etag = lineIndex
	return true
}
func (r *backendResponse_) checkExpires(headerLine *pair, lineIndex uint8) bool { // Expires = HTTP-date
	return r._checkHTTPDate(headerLine, lineIndex, &r.indexes.expires, &r.unixTimes.expires)
}
func (r *backendResponse_) checkLastModified(headerLine *pair, lineIndex uint8) bool { // Last-Modified = HTTP-date
	return r._checkHTTPDate(headerLine, lineIndex, &r.indexes.lastModified, &r.unixTimes.lastModified)
}
func (r *backendResponse_) checkLocation(headerLine *pair, lineIndex uint8) bool { // Location = URI-reference
	// TODO: check
	r.indexes.location = lineIndex
	return true
}
func (r *backendResponse_) checkRetryAfter(headerLine *pair, lineIndex uint8) bool { // Retry-After = HTTP-date / delay-seconds
	// TODO: check
	r.indexes.retryAfter = lineIndex
	return true
}
func (r *backendResponse_) checkServer(headerLine *pair, lineIndex uint8) bool { // Server = product *( RWS ( product / comment ) )
	// TODO: check
	r.indexes.server = lineIndex
	return true
}
func (r *backendResponse_) checkSetCookie(headerLine *pair, lineIndex uint8) bool { // Set-Cookie = set-cookie-string
	// set-cookie-string = cookie-pair *( ";" SP cookie-av )
	// cookie-pair = token "=" cookie-value
	// cookie-value = *cookie-octet / ( DQUOTE *cookie-octet DQUOTE )
	// cookie-octet = %x21 / %x23-2B / %x2D-3A / %x3C-5B / %x5D-7E
	// cookie-av = expires-av / max-age-av / domain-av / path-av / secure-av / httponly-av / samesite-av / extension-av
	// expires-av = "Expires=" sane-cookie-date
	// max-age-av = "Max-Age=" non-zero-digit *DIGIT
	// domain-av = "Domain=" domain-value
	// path-av = "Path=" path-value
	// secure-av = "Secure"
	// httponly-av = "HttpOnly"
	// samesite-av = "SameSite=" samesite-value
	// extension-av = <any CHAR except CTLs or ";">
	return true
}

var ( // minimal perfect hash table for important response header fields
	backendResponseImportantHeaderFieldTable = [20]struct {
		fdesc // allowQuote, allowEmpty, allowParam, hasComment
		check func(*backendResponse_, []pair, uint8, uint8) bool
	}{ // accept accept-encoding accept-ranges allow alt-svc cache-control cache-status cdn-cache-control connection content-encoding content-language keep-alive proxy-authenticate proxy-connection trailer transfer-encoding upgrade vary via www-authenticate
		0:  {fdesc{hashAccept, false, true, true, false, bytesAccept}, (*backendResponse_).checkAccept},
		1:  {fdesc{hashAltSvc, false, false, true, false, bytesAltSvc}, (*backendResponse_).checkAltSvc},
		2:  {fdesc{hashContentEncoding, false, false, false, false, bytesContentEncoding}, (*backendResponse_).checkContentEncoding},
		3:  {fdesc{hashVia, false, false, false, true, bytesVia}, (*backendResponse_).checkVia},
		4:  {fdesc{hashAcceptEncoding, false, true, true, false, bytesAcceptEncoding}, (*backendResponse_).checkAcceptEncoding},
		5:  {fdesc{hashKeepAlive, false, false, false, false, bytesKeepAlive}, (*backendResponse_).checkKeepAlive},
		6:  {fdesc{hashCDNCacheControl, false, false, false, false, bytesCDNCacheControl}, (*backendResponse_).checkCDNCacheControl},
		7:  {fdesc{hashCacheStatus, false, false, true, false, bytesCacheStatus}, (*backendResponse_).checkCacheStatus},
		8:  {fdesc{hashConnection, false, false, false, false, bytesConnection}, (*backendResponse_).checkConnection},
		9:  {fdesc{hashAllow, false, true, false, false, bytesAllow}, (*backendResponse_).checkAllow},
		10: {fdesc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*backendResponse_).checkUpgrade},
		11: {fdesc{hashContentLanguage, false, false, false, false, bytesContentLanguage}, (*backendResponse_).checkContentLanguage},
		12: {fdesc{hashProxyConnection, false, false, false, false, bytesProxyConnection}, (*backendResponse_).checkProxyConnection},
		13: {fdesc{hashWWWAuthenticate, false, false, false, false, bytesWWWAuthenticate}, (*backendResponse_).checkWWWAuthenticate},
		14: {fdesc{hashTrailer, false, false, false, false, bytesTrailer}, (*backendResponse_).checkTrailer},
		15: {fdesc{hashCacheControl, false, false, false, false, bytesCacheControl}, (*backendResponse_).checkCacheControl},
		16: {fdesc{hashProxyAuthenticate, false, false, false, false, bytesProxyAuthenticate}, (*backendResponse_).checkProxyAuthenticate},
		17: {fdesc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*backendResponse_).checkTransferEncoding}, // deliberately false
		18: {fdesc{hashVary, false, false, false, false, bytesVary}, (*backendResponse_).checkVary},
		19: {fdesc{hashAcceptRanges, false, false, false, false, bytesAcceptRanges}, (*backendResponse_).checkAcceptRanges},
	}
	backendResponseImportantHeaderFieldFind = func(nameHash uint16) int {
		return (964916190 / int(nameHash)) % len(backendResponseImportantHeaderFieldTable)
	}
)

func (r *backendResponse_) checkAcceptRanges(subLines []pair, subFrom uint8, subEdge uint8) bool { // Accept-Ranges = 1#range-unit
	if subFrom == subEdge {
		r.headResult, r.failReason = StatusBadRequest, "accept-ranges = 1#range-unit"
		return false
	}
	if r.zones.acceptRanges.isEmpty() {
		r.zones.acceptRanges.from = subFrom
	}
	r.zones.acceptRanges.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		subData := subLines[i].dataAt(r.input)
		bytesToLower(subData) // range unit names are case-insensitive
		if bytes.Equal(subData, bytesBytes) {
			r.acceptBytes = true
		} else {
			// Ignore
		}
	}
	return true
}
func (r *backendResponse_) checkAllow(subLines []pair, subFrom uint8, subEdge uint8) bool { // Allow = #method
	if r.zones.allow.isEmpty() {
		r.zones.allow.from = subFrom
	}
	r.zones.allow.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	r.hasAllow = true
	return true
}
func (r *backendResponse_) checkAltSvc(subLines []pair, subFrom uint8, subEdge uint8) bool { // Alt-Svc = clear / 1#alt-value
	if subFrom == subEdge {
		r.headResult, r.failReason = StatusBadRequest, "alt-svc = clear / 1#alt-value"
		return false
	}
	if r.zones.altSvc.isEmpty() {
		r.zones.altSvc.from = subFrom
	}
	r.zones.altSvc.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO: check syntax
	}
	return true
}
func (r *backendResponse_) checkCacheControl(subLines []pair, subFrom uint8, subEdge uint8) bool { // Cache-Control = #cache-directive
	if r.zCacheControl.isEmpty() {
		r.zCacheControl.from = subFrom
	}
	r.zCacheControl.edge = subEdge
	// cache-directive = token [ "=" ( token / quoted-string ) ]
	for i := subFrom; i < subEdge; i++ {
		// TODO: check for backend
	}
	return true
}
func (r *backendResponse_) checkCacheStatus(subLines []pair, subFrom uint8, subEdge uint8) bool { // ?
	if r.zones.cacheStatus.isEmpty() {
		r.zones.cacheStatus.from = subFrom
	}
	r.zones.cacheStatus.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO
	}
	return true
}
func (r *backendResponse_) checkCDNCacheControl(subLines []pair, subFrom uint8, subEdge uint8) bool { // ?
	if r.zones.cdnCacheControl.isEmpty() {
		r.zones.cacheStatus.from = subFrom
	}
	r.zones.cdnCacheControl.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO
	}
	return true
}
func (r *backendResponse_) checkProxyAuthenticate(subLines []pair, subFrom uint8, subEdge uint8) bool { // Proxy-Authenticate = #challenge
	if r.zones.proxyAuthenticate.isEmpty() {
		r.zones.cacheStatus.from = subFrom
	}
	r.zones.proxyAuthenticate.edge = subEdge
	// TODO; use r._checkChallenge
	return true
}
func (r *backendResponse_) checkUpgrade(subLines []pair, subFrom uint8, subEdge uint8) bool { // Upgrade = #protocol
	if r.httpVersion >= Version2 {
		r.headResult, r.failReason = StatusBadRequest, "upgrade is not supported in http/2 and http/3"
		return false
	}
	if r.zUpgrade.isEmpty() {
		r.zUpgrade.from = subFrom
	}
	r.zUpgrade.edge = subEdge
	// TODO: what about upgrade: websocket?
	r.headResult, r.failReason = StatusBadRequest, "upgrade is not supported in exchan mode"
	return false
}
func (r *backendResponse_) checkVary(subLines []pair, subFrom uint8, subEdge uint8) bool { // Vary = #( "*" / field-name )
	if r.zones.vary.isEmpty() {
		r.zones.vary.from = subFrom
	}
	r.zones.vary.edge = subEdge
	for i := subFrom; i < subEdge; i++ {
		// TODO
	}
	return true
}
func (r *backendResponse_) checkWWWAuthenticate(subLines []pair, subFrom uint8, subEdge uint8) bool { // WWW-Authenticate = #challenge
	if r.zones.wwwAuthenticate.isEmpty() {
		r.zones.cacheStatus.from = subFrom
	}
	r.zones.wwwAuthenticate.edge = subEdge
	// TODO; use r._checkChallenge
	return true
}
func (r *backendResponse_) _checkChallenge(subLines []pair, subFrom uint8, subEdge uint8) bool { // challenge = auth-scheme [ 1*SP ( token68 / [ auth-param *( OWS "," OWS auth-param ) ] ) ]
	for i := subFrom; i < subEdge; i++ {
		// TODO
	}
	return true
}

func (r *backendResponse_) parseSetCookie() bool {
	// TODO
	return false
}

func (r *backendResponse_) unsafeDate() []byte {
	if r.iDate == 0 {
		return nil
	}
	return r.primes[r.iDate].valueAt(r.input)
}
func (r *backendResponse_) unsafeLastModified() []byte {
	if r.indexes.lastModified == 0 {
		return nil
	}
	return r.primes[r.indexes.lastModified].valueAt(r.input)
}

func (r *backendResponse_) proxyUnsetXXX() {
	// TODO
}
func (r *backendResponse_) proxyDelHopFieldLines(kind int8) {
	// Currently nothing.
}

func (r *backendResponse_) HasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified) responses do not include content.
	if r.status < StatusOK || r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might be of zero length.
	return r.contentSize >= 0 || r.IsVague()
}
func (r *backendResponse_) Content() string       { return string(r.unsafeContent()) }
func (r *backendResponse_) UnsafeContent() []byte { return r.unsafeContent() }

func (r *backendResponse_) examineTail() bool {
	for i := r.trailerLines.from; i < r.trailerLines.edge; i++ {
		if !r.applyTrailerLine(i) {
			// r.bodyResult is set.
			return false
		}
	}
	return true
}
func (r *backendResponse_) applyTrailerLine(lineIndex uint8) bool {
	//trailerLine := &r.primes[lineIndex]
	// TODO: Pseudo-header fields MUST NOT appear in a trailer section.
	return true
}

// BackendRequest is the backend-side http request.
type BackendRequest interface { // for *backend[1-3]Request
	proxySetMethodURI(method []byte, uri []byte, hasContent bool) bool
	proxySetAuthority(hostname []byte, colonport []byte) bool
	proxyCopyCookies(servReq ServerRequest) bool // NOTE: HTTP 1.x/2/3 have different requirements on the "cookie" header field
	proxyCopyHeaderLines(servReq ServerRequest, proxyConfig *HTTPProxyConfig) bool
	proxyPassMessage(servReq ServerRequest) error                 // pass content to backend directly
	proxyPostMessage(foreContent any, foreHasTrailers bool) error // post held content to backend
	proxyCopyTrailerLines(servReq ServerRequest, proxyConfig *HTTPProxyConfig) bool
	isVague() bool
	endVague() error
}

// backendRequest_ is a parent.
type backendRequest_ struct { // for backend[1-3]Request. outgoing, needs building
	// Mixins
	_httpOut_ // outgoing http request
	// Assocs
	response BackendResponse // the corresponding response
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	unixTimes struct { // in seconds
		ifModifiedSince   int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
		ifUnmodifiedSince int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
	}
	// Stream states (zeros)
	_backendRequest0 // all values in this struct must be zero by default!
}
type _backendRequest0 struct { // for fast reset, entirely
	addTETrailers bool // add "te: trailers" in finalizeHeaders()?
	indexes       struct {
		host              uint8
		ifModifiedSince   uint8
		ifUnmodifiedSince uint8
		ifRange           uint8
	}
}

func (r *backendRequest_) onUse(httpVersion uint8) { // for non-zeros
	r._httpOut_.onUse(httpVersion, true) // as request

	r.unixTimes.ifModifiedSince = -1   // -1 means not set
	r.unixTimes.ifUnmodifiedSince = -1 // -1 means not set
}
func (r *backendRequest_) onEnd() { // for zeros
	r._backendRequest0 = _backendRequest0{}

	r._httpOut_.onEnd()
}

func (r *backendRequest_) Response() BackendResponse { return r.response }

func (r *backendRequest_) setScheme(scheme []byte) bool { // used by http/2 and http/3 only. http/1.x doesn't use this!
	// TODO: copy `:scheme $scheme` to r.output
	return false
}
func (r *backendRequest_) controlData() []byte { return r.output[0:r.controlEdge] } // TODO: maybe we need a struct type to represent pseudo header fields?

func (r *backendRequest_) SetIfModifiedSince(since int64) bool {
	return r._setUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince, since)
}
func (r *backendRequest_) SetIfUnmodifiedSince(since int64) bool {
	return r._setUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince, since)
}

func (r *backendRequest_) beforeSend() {} // revising is not supported in backend side.
func (r *backendRequest_) doSend() error { // revising is not supported in backend side.
	return r.out.sendChain()
}

func (r *backendRequest_) beforeEcho() {} // revising is not supported in backend side.
func (r *backendRequest_) doEcho() error { // revising is not supported in backend side.
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	r.chain.PushTail(&r.piece)
	defer r.chain.free()
	return r.out.echoChain()
}
func (r *backendRequest_) endVague() error { // revising is not supported in backend side.
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	return r.out.finalizeVague()
}

var ( // minimal perfect hash table for request critical header fields
	backendRequestCriticalHeaderFieldTable = [12]struct {
		hash uint16
		name []byte
		fAdd func(*backendRequest_, []byte) (ok bool)
		fDel func(*backendRequest_) (deleted bool)
	}{ // connection content-length content-type cookie date host if-modified-since if-range if-unmodified-since transfer-encoding upgrade via
		0:  {hashContentLength, bytesContentLength, nil, nil}, // restricted. added at finalizeHeaders()
		1:  {hashConnection, bytesConnection, nil, nil},       // restricted. added at finalizeHeaders()
		2:  {hashIfRange, bytesIfRange, (*backendRequest_)._insertIfRange, (*backendRequest_)._removeIfRange},
		3:  {hashUpgrade, bytesUpgrade, nil, nil}, // restricted. not allowed to change the protocol. may be added if webSocket?
		4:  {hashIfModifiedSince, bytesIfModifiedSince, (*backendRequest_)._insertIfModifiedSince, (*backendRequest_)._removeIfModifiedSince},
		5:  {hashIfUnmodifiedSince, bytesIfUnmodifiedSince, (*backendRequest_)._insertIfUnmodifiedSince, (*backendRequest_)._removeIfUnmodifiedSince},
		6:  {hashHost, bytesHost, (*backendRequest_)._insertHost, (*backendRequest_)._removeHost},
		7:  {hashTransferEncoding, bytesTransferEncoding, nil, nil}, // restricted. added at finalizeHeaders() if needed
		8:  {hashContentType, bytesContentType, (*backendRequest_)._insertContentType, (*backendRequest_)._removeContentType},
		9:  {hashCookie, bytesCookie, nil, nil}, // restricted. added separately
		10: {hashDate, bytesDate, (*backendRequest_)._insertDate, (*backendRequest_)._removeDate},
		11: {hashVia, bytesVia, nil, nil}, // restricted. added if needed when acting as a proxy
	}
	backendRequestCriticalHeaderFieldFind = func(nameHash uint16) int {
		return (645048 / int(nameHash)) % len(backendRequestCriticalHeaderFieldTable)
	}
)

func (r *backendRequest_) insertHeader(nameHash uint16, name []byte, value []byte) bool {
	h := &backendRequestCriticalHeaderFieldTable[backendRequestCriticalHeaderFieldFind(nameHash)]
	if h.hash == nameHash && bytes.Equal(h.name, name) {
		if h.fAdd == nil { // mainly because this header field is restricted to insert
			return true // pretend to be successful
		}
		return h.fAdd(r, value)
	}
	return r.out.addHeader(name, value)
}
func (r *backendRequest_) _insertHost(host []byte) (ok bool) {
	return r._appendSingleton(&r.indexes.host, bytesHost, host)
}
func (r *backendRequest_) _insertIfRange(ifRange []byte) (ok bool) {
	return r._appendSingleton(&r.indexes.ifRange, bytesIfRange, ifRange)
}
func (r *backendRequest_) _insertIfModifiedSince(since []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince, bytesIfModifiedSince, since)
}
func (r *backendRequest_) _insertIfUnmodifiedSince(since []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince, bytesIfUnmodifiedSince, since)
}

func (r *backendRequest_) removeHeader(nameHash uint16, name []byte) bool {
	h := &backendRequestCriticalHeaderFieldTable[backendRequestCriticalHeaderFieldFind(nameHash)]
	if h.hash == nameHash && bytes.Equal(h.name, name) {
		if h.fDel == nil { // mainly because this header field is restricted to remove
			return true // pretend to be successful
		}
		return h.fDel(r)
	}
	return r.out.delHeader(name)
}
func (r *backendRequest_) _removeHost() (deleted bool) {
	return r._deleteSingleton(&r.indexes.host)
}
func (r *backendRequest_) _removeIfRange() (deleted bool) {
	return r._deleteSingleton(&r.indexes.ifRange)
}
func (r *backendRequest_) _removeIfModifiedSince() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince)
}
func (r *backendRequest_) _removeIfUnmodifiedSince() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince)
}

func (r *backendRequest_) proxyPassMessage(servReq ServerRequest) error {
	return r._proxyPassMessage(servReq)
}
func (r *backendRequest_) proxyCopyHeaderLines(servReq ServerRequest, proxyConfig *HTTPProxyConfig) bool {
	servReq.proxyDelHopHeaderFields()

	// Copy control (:method, :path, :authority, :scheme)
	uri := servReq.UnsafeURI()
	if servReq.IsAsteriskOptions() { // OPTIONS *
		// RFC 9112 (3.2.4):
		// If a proxy receives an OPTIONS request with an absolute-form of request-target in which the URI has an empty path and no query component,
		// then the last proxy on the request chain MUST send a request-target of "*" when it forwards the request to the indicated origin server.
		uri = bytesAsterisk
	}
	if !r.out.(BackendRequest).proxySetMethodURI(servReq.UnsafeMethod(), uri, servReq.HasContent()) {
		return false
	}
	if len(proxyConfig.Hostname) != 0 || len(proxyConfig.Colonport) != 0 { // custom authority (hostname or colonport)
		servReq.proxyUnsetHost()
		var (
			hostname  []byte
			colonport []byte
		)
		if len(proxyConfig.Hostname) == 0 { // no custom hostname
			hostname = servReq.UnsafeHostname()
		} else {
			hostname = proxyConfig.Hostname
		}
		if len(proxyConfig.Colonport) == 0 { // no custom colonport
			colonport = servReq.UnsafeColonport()
		} else {
			colonport = proxyConfig.Colonport
		}
		if !r.out.(BackendRequest).proxySetAuthority(hostname, colonport) {
			return false
		}
	}
	if r.httpVersion >= Version2 {
		var scheme []byte
		if r.stream.TLSMode() {
			scheme = bytesSchemeHTTPS
		} else {
			scheme = bytesSchemeHTTP
		}
		if !r.setScheme(scheme) {
			return false
		}
	} else {
		// we have no way to set scheme in HTTP/1.x unless we use absolute-form, which is a risk as some servers may not support it.
	}

	// Copy selective forbidden header fields (including cookie) from servReq
	if servReq.HasCookies() && !r.out.(BackendRequest).proxyCopyCookies(servReq) {
		return false
	}
	if !r.out.addHeader(bytesVia, proxyConfig.InboundViaName) { // an HTTP-to-HTTP gateway MUST send an appropriate Via header field in each inbound request message
		return false
	}
	if servReq.AcceptTrailers() {
		r.addTETrailers = true
	}

	// Copy added header fields
	for headerName, vHeaderValue := range proxyConfig.AddRequestHeaders {
		var headerValue []byte
		if vHeaderValue.IsVariable() {
			headerValue = vHeaderValue.BytesVar(servReq)
		} else if v, ok := vHeaderValue.Bytes(); ok {
			headerValue = v
		} else {
			// Invalid values are treated as empty
		}
		if !r.out.addHeader(ConstBytes(headerName), headerValue) {
			return false
		}
	}

	// Copy remaining header fields from servReq
	if !servReq.proxyWalkHeaderLines(r.out, func(out httpOut, headerLine *pair, headerName []byte, lineValue []byte) bool {
		if false { // TODO: are there any special header fields that should be copied directly?
			return out.addHeader(headerName, lineValue)
		} else {
			return out.insertHeader(headerLine.nameHash, headerName, lineValue) // some header fields (e.g. "connection") are restricted
		}
	}) {
		return false
	}

	// This must be placed at the end so we can delete some header fields forcely.
	for _, headerName := range proxyConfig.DelRequestHeaders {
		r.out.delHeader(headerName)
	}

	return true
}
func (r *backendRequest_) proxyCopyTrailerLines(servReq ServerRequest, proxyConfig *HTTPProxyConfig) bool {
	return servReq.proxyWalkTrailerLines(r.out, func(out httpOut, trailerLine *pair, trailerName []byte, lineValue []byte) bool {
		return out.addTrailer(trailerName, lineValue)
	})
}

// BackendSocket is the backend-side webSocket.
type BackendSocket interface { // for *backend[1-3]Socket
	Read(dst []byte) (int, error)
	Write(src []byte) (int, error)
	Close() error
}

// backendSocket_ is a parent.
type backendSocket_ struct { // for backend[1-3]Socket. incoming and outgoing
	// Mixins
	_httpSocket_
	// Assocs
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	_backendSocket0 // all values in this struct must be zero by default!
}
type _backendSocket0 struct { // for fast reset, entirely
}

func (s *backendSocket_) onUse() {
	const asServer = false
	s._httpSocket_.onUse(asServer)
}
func (s *backendSocket_) onEnd() {
	s._backendSocket0 = _backendSocket0{}

	s._httpSocket_.onEnd()
}

func (s *backendSocket_) backendTodo() {
}
