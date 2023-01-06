// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General HTTP client implementation.

package internal

import (
	"bytes"
	"sync/atomic"
	"time"
)

// httpClient is the interface for http outgates and http backends.
type httpClient interface {
	client
	streamHolder
	contentSaver
	MaxContentSize() int64
}

// httpClient_ is a mixin for httpOutgate_ and httpBackend_.
type httpClient_ struct {
	// Mixins
	streamHolder_
	contentSaver_ // so responses can save their large contents in local file system.
	// States
	maxContentSize int64
}

func (h *httpClient_) onCreate() {
}

func (h *httpClient_) onConfigure(shell Component, clientName string) {
	h.streamHolder_.onConfigure(shell, 1000)
	h.contentSaver_.onConfigure(shell, TempDir()+"/"+clientName+"/"+shell.Name())
	// maxContentSize
	shell.ConfigureInt64("maxContentSize", &h.maxContentSize, func(value int64) bool { return value > 0 }, _1T)
}
func (h *httpClient_) onPrepare(shell Component) {
	h.streamHolder_.onPrepare(shell)
	h.contentSaver_.onPrepare(shell, 0755)
}

func (h *httpClient_) MaxContentSize() int64 { return h.maxContentSize }

// httpOutgate_ is the mixin for HTTP[1-3]Outgate.
type httpOutgate_ struct {
	// Mixins
	client_
	httpClient_
	// States
}

func (f *httpOutgate_) onCreate(name string, stage *Stage) {
	f.client_.onCreate(name, stage)
	f.httpClient_.onCreate()
}

func (f *httpOutgate_) onConfigure(shell Component) {
	f.client_.onConfigure()
	f.httpClient_.onConfigure(shell, "outgates")
}
func (f *httpOutgate_) onPrepare(shell Component) {
	f.client_.onPrepare()
	f.httpClient_.onPrepare(shell)
}

// httpBackend_ is the mixin for HTTP[1-3]Backend.
type httpBackend_[N node] struct {
	// Mixins
	backend_[N]
	httpClient_
	loadBalancer_
	// States
	health any // TODO
}

func (b *httpBackend_[N]) onCreate(name string, stage *Stage, creator interface{ createNode(id int32) N }) {
	b.backend_.onCreate(name, stage, creator)
	b.httpClient_.onCreate()
	b.loadBalancer_.init()
}

func (b *httpBackend_[N]) onConfigure(shell Component) {
	b.backend_.onConfigure()
	b.httpClient_.onConfigure(shell, "backends")
	b.loadBalancer_.onConfigure(shell)
}
func (b *httpBackend_[N]) onPrepare(shell Component, numNodes int) {
	b.backend_.onPrepare()
	b.httpClient_.onPrepare(shell)
	b.loadBalancer_.onPrepare(numNodes)
}

// hConn is the interface for *H[1-3]Conn.
type hConn interface {
	getClient() httpClient
	isBroken() bool
	markBroken()
	makeTempName(p []byte, stamp int64) (from int, edge int) // small enough to be placed in tinyBuffer() of stream
}

// hConn_ is the mixin for H[1-3]Conn.
type hConn_ struct {
	// Mixins
	conn_
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
	counter     atomic.Int64 // used to make temp name
	usedStreams atomic.Int32 // how many streams has been used?
	broken      atomic.Bool  // is conn broken?
}

func (c *hConn_) onGet(id int64, client httpClient) {
	c.conn_.onGet(id, client)
}
func (c *hConn_) onPut() {
	c.conn_.onPut()
	c.counter.Store(0)
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *hConn_) getClient() httpClient { return c.client.(httpClient) }

func (c *hConn_) isBroken() bool { return c.broken.Load() }
func (c *hConn_) markBroken()    { c.broken.Store(true) }

func (c *hConn_) makeTempName(p []byte, stamp int64) (from int, edge int) {
	return makeTempName(p, int64(c.client.Stage().ID()), c.id, stamp, c.counter.Add(1))
}

func (c *hConn_) reachLimit() bool {
	return c.usedStreams.Add(1) > c.getClient().MaxStreamsPerConn()
}

// hStream_ is the mixin for H[1-3]Stream.
type hStream_ struct {
	// Mixins
	stream_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *hStream_) callTCPTun() {
	// TODO
}
func (s *hStream_) callUDPTun() {
	// TODO
}
func (s *hStream_) callSocket() {
	// TODO
}

// request is the client-side HTTP request and the interface for *H[1-3]Request.
type request interface {
	Response() response
	SetMaxSendTimeout(timeout time.Duration) // to defend against bad server

	setControl(method []byte, uri []byte, hasContent bool) bool
	addHeader(name []byte, value []byte) bool
	copyCookies(req Request) bool
}

// hRequest_ is the mixin for H[1-3]Request.
type hRequest_ struct {
	// Mixins
	httpOutMessage_
	// Assocs
	response response // the corresponding response
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	hRequest0_ // all values must be zero by default in this struct!
}
type hRequest0_ struct { // for fast reset, entirely
	cookieCopied bool // is cookie header copied?
}

func (r *hRequest_) onUse() { // for non-zeros
	r.httpOutMessage_.onUse(true)
}
func (r *hRequest_) onEnd() { // for zeros
	r.hRequest0_ = hRequest0_{}
	r.httpOutMessage_.onEnd()
}

func (r *hRequest_) Response() response { return r.response }

func (r *hRequest_) send() error {
	return r.shell.sendChain(r.content)
}

func (r *hRequest_) checkPush() error {
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
	return r.shell.pushHeaders()
}
func (r *hRequest_) push(chunk *Block) error {
	var curChain Chain
	curChain.PushTail(chunk)
	defer curChain.free()

	if r.stream.isBroken() {
		return httpWriteBroken
	}
	return r.shell.pushChain(curChain)
}

func (r *hRequest_) copyHead(req Request) bool { // used by proxies
	var uri []byte
	if req.IsAsteriskOptions() { // OPTIONS *
		// RFC 9112 (3.2.4):
		// If a proxy receives an OPTIONS request with an absolute-form of request-target in which the URI has an empty path and no query component,
		// then the last proxy on the request chain MUST send a request-target of "*" when it forwards the request to the indicated origin server.

		// Note, even target form is asterisk-form, not absolute-form, req.uri is still empty in our implementation, not "*". So just use "*".
		uri = httpBytesAsterisk
	} else {
		uri = req.UnsafeURI()
	}
	if !r.shell.(request).setControl(req.UnsafeMethod(), uri, req.HasContent()) {
		return false
	}
	req.delHopHeaders()

	// copy critical headers from req
	if req.AcceptTrailers() { // te: trailers
		if !r.shell.addHeader(httpBytesTE, httpBytesTrailers) {
			return false
		}
	}
	req.delCriticalHeaders()

	if req.IsAbsoluteForm() {
		// When a proxy receives a request with an absolute-form of request-target, the proxy MUST ignore the received Host header
		// field (if any) and instead replace it with the host information of the request-target. A proxy that forwards such a request
		// MUST generate a new Host field value based on the received request-target rather than forward the received Host field value.
		req.delHost()
		if !r.shell.addHeader(httpBytesHost, req.UnsafeAuthority()) {
			return false
		}
	}
	// copy remaining headers from req
	if !req.walkHeaders(func(name []byte, value []byte) bool {
		return r.shell.addHeader(name, value)
	}, true) { // for proxy
		return false
	}
	if req.HasCookies() && !r.shell.(request).copyCookies(req) {
		return false
	}
	return true
}
func (r *hRequest_) pass(req httpInMessage) error { // used by proxies.
	return r.doPass(req, false) // no revisers in client side
}

func (r *hRequest_) finishChunked() error {
	if r.stream.isBroken() {
		return httpWriteBroken
	}
	return r.shell.finalizeChunked()
}

func (r *hRequest_) isCrucialField(hash uint16, name []byte) bool {
	/*
		for _, field := range hRequestCrucialFieldTable {
			if field.hash == hash && bytes.Equal(field.name, name) {
				return true
			}
		}
	*/
	return false
}

var ( // perfect hash table for request crucial fields
	hRequestCrucialFieldNames = []byte("connection content-length transfer-encoding cookie")
	hRequestCrucialFieldTable = [4]struct { // TODO: perfect hashing
		hash uint16
		from uint8
		edge uint8
		fAdd func()
		fDel func()
	}{
		0: {httpHashConnection, 0, 1, nil, nil},
		1: {httpHashContentLength, 2, 3, nil, nil},
		2: {httpHashTransferEncoding, 4, 5, nil, nil},
		3: {httpHashCookie, 6, 7, nil, nil},
	}
	hRequestCrucialFieldFind = func(hash uint16) int { return 1 }
)

func (r *hRequest_) addConnection() {
}
func (r *hRequest_) delConnection() {
}

func (r *hRequest_) addContentLength() {
}
func (r *hRequest_) delContentLength() {
}

func (r *hRequest_) addTransferEncoding() {
}
func (r *hRequest_) delTransferEncoding() {
}

func (r *hRequest_) addCookie() {
}
func (r *hRequest_) delCookie() {
}

// response is the client-side HTTP response and interface for *H[1-3]Response.
type response interface {
	Status() int16
	ContentSize() int64
	UnsafeContentType() []byte
	SetMaxRecvTimeout(timeout time.Duration) // to defend against bad server
	HasTrailers() bool

	unsafeDate() []byte
	unsafeETag() []byte
	unsafeLastModified() []byte
	delCriticalHeaders()
	delHopHeaders()
	delHopTrailers()
	walkHeaders(fn func(name []byte, value []byte) bool, forProxy bool) bool
	walkTrailers(fn func(name []byte, value []byte) bool, forProxy bool) bool
	recvContent(retain bool) any
	readContent() (p []byte, err error)
}

// hResponse_ is the mixin for H[1-3]Response.
type hResponse_ struct {
	// Mixins
	httpInMessage_
	// Stream states (buffers)
	stockSetCookies [4]setCookie // for r.setCookies
	// Stream states (controlled)
	// Stream states (non-zeros)
	setCookies []setCookie // hold setCookies->r.input. [<r.stockSetCookies>/(make=16/128)]
	// Stream states (zeros)
	hResponse0_ // all values must be zero by default in this struct!
}
type hResponse0_ struct { // for fast reset, entirely
	status           int16    // 200, 302, 404, ...
	dateTime         int64    // parsed unix timestamp of date
	lastModifiedTime int64    // parsed unix timestamp of last-modified
	expiresTime      int64    // parsed unix timestamp of expires
	cacheControl     struct { // the cache-control info
		noCache         bool  // ...
		noStore         bool  // ...
		noTransform     bool  // ...
		public          bool  // ...
		private         bool  // ...
		mustRevalidate  bool  // ...
		mustUnderstand  bool  // ...
		proxyRevalidate bool  // ...
		maxAge          int32 // ...
		sMaxAge         int32 // ...
	}
	indexes struct { // indexes of some selected headers, for fast accessing
		server       uint8 // server header ->r.input
		date         uint8 // date header ->r.input
		lastModified uint8 // last-modified header ->r.input
		expires      uint8 // expires header ->r.input
		etag         uint8 // etag header ->r.input
		acceptRanges uint8 // accept-ranges header ->r.input
		location     uint8 // location header ->r.input
	}
}

func (r *hResponse_) onUse() { // for non-zeros
	r.httpInMessage_.onUse(true)
	r.setCookies = r.stockSetCookies[0:0:cap(r.stockSetCookies)] // use append()
}
func (r *hResponse_) onEnd() { // for zeros
	if cap(r.setCookies) != cap(r.stockSetCookies) {
		// put?
		r.setCookies = nil
	}
	r.hResponse0_ = hResponse0_{}
	r.httpInMessage_.onEnd()
}

func (r *hResponse_) Status() int16 { return r.status }

func (r *hResponse_) applyHeader(header *pair) bool {
	headerName := header.nameAt(r.input)
	if h := &hResponseMultipleHeaderTable[hResponseMultipleHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(hResponseMultipleHeaderNames[h.from:h.edge], headerName) {
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
	} else { // single-value response header
		if !r.addHeader(header) {
			// r.headResult is set.
			return false
		}
		if h := &hResponseCriticalHeaderTable[hResponseCriticalHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(hResponseCriticalHeaderNames[h.from:h.edge], headerName) {
			if h.check != nil && !h.check(r, header, r.headers.edge-1) {
				// r.headResult is set.
				return false
			}
		}
	}
	return true
}

var ( // perfect hash table for response multiple headers
	hResponseMultipleHeaderNames = []byte("accept-encoding accept-ranges allow cache-control connection content-encoding content-language proxy-authenticate trailer transfer-encoding upgrade vary via www-authenticate")
	hResponseMultipleHeaderTable = [14]struct {
		hash  uint16
		from  uint8
		edge  uint8
		must  bool // true if 1#, false if #
		check func(*hResponse_, uint8, uint8) bool
	}{
		0:  {httpHashVary, 148, 152, false, nil},
		1:  {httpHashConnection, 50, 60, false, (*hResponse_).checkConnection},
		2:  {httpHashAllow, 30, 35, false, nil},
		3:  {httpHashTrailer, 114, 121, false, nil},
		4:  {httpHashVia, 153, 156, false, nil},
		5:  {httpHashContentEncoding, 61, 77, false, (*hResponse_).checkContentEncoding},
		6:  {httpHashAcceptRanges, 16, 29, false, nil},
		7:  {httpHashProxyAuthenticate, 95, 113, false, nil},
		8:  {httpHashTransferEncoding, 122, 139, false, (*hResponse_).checkTransferEncoding},
		9:  {httpHashCacheControl, 36, 49, false, (*hResponse_).checkCacheControl},
		10: {httpHashContentLanguage, 78, 94, false, nil},
		11: {httpHashWWWAuthenticate, 157, 173, false, nil},
		12: {httpHashAcceptEncoding, 0, 15, false, nil},
		13: {httpHashUpgrade, 140, 147, false, (*hResponse_).checkUpgrade},
	}
	hResponseMultipleHeaderFind = func(hash uint16) int { return (4114134 / int(hash)) % 14 }
)

func (r *hResponse_) checkCacheControl(from uint8, edge uint8) bool {
	// Cache-Control   = 1#cache-directive
	// cache-directive = token [ "=" ( token / quoted-string ) ]
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *hResponse_) checkTransferEncoding(from uint8, edge uint8) bool {
	if r.status < StatusOK || r.status == StatusNoContent {
		r.headResult, r.headReason = StatusBadRequest, "transfer-encoding is not allowed in 1xx and 204 responses"
		return false
	}
	if r.status == StatusNotModified {
		// TODO
	}
	return r.httpInMessage_.checkTransferEncoding(from, edge)
}
func (r *hResponse_) checkUpgrade(from uint8, edge uint8) bool {
	r.headResult, r.headReason = StatusBadRequest, "upgrade is not supported in normal mode"
	return false
}

var ( // perfect hash table for response critical headers
	hResponseCriticalHeaderNames = []byte("content-length content-range content-type date etag expires last-modified location server set-cookie")
	hResponseCriticalHeaderTable = [10]struct {
		hash  uint16
		from  uint8
		edge  uint8
		check func(*hResponse_, *pair, uint8) bool
	}{
		0: {httpHashLocation, 74, 82, (*hResponse_).checkLocation},
		1: {httpHashContentRange, 15, 28, (*hResponse_).checkContentRange},
		2: {httpHashLastModified, 60, 73, (*hResponse_).checkLastModified},
		3: {httpHashServer, 83, 89, (*hResponse_).checkServer},
		4: {httpHashContentType, 29, 41, (*hResponse_).checkContentType},
		5: {httpHashETag, 47, 51, (*hResponse_).checkETag},
		6: {httpHashDate, 42, 46, (*hResponse_).checkDate},
		7: {httpHashContentLength, 0, 14, (*hResponse_).checkContentLength},
		8: {httpHashSetCookie, 90, 100, (*hResponse_).checkSetCookie},
		9: {httpHashExpires, 52, 59, (*hResponse_).checkExpires},
	}
	hResponseCriticalHeaderFind = func(hash uint16) int { return (68805 / int(hash)) % 10 }
)

func (r *hResponse_) checkContentRange(header *pair, index uint8) bool {
	// TODO
	return true
}
func (r *hResponse_) checkDate(header *pair, index uint8) bool {
	// Date = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.date, &r.dateTime)
}
func (r *hResponse_) checkETag(header *pair, index uint8) bool {
	r.indexes.etag = index
	return true
}
func (r *hResponse_) checkExpires(header *pair, index uint8) bool {
	// Expires = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.expires, &r.expiresTime)
}
func (r *hResponse_) checkLastModified(header *pair, index uint8) bool {
	// Last-Modified = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.lastModified, &r.lastModifiedTime)
}
func (r *hResponse_) checkLocation(header *pair, index uint8) bool {
	r.indexes.location = index
	return true
}
func (r *hResponse_) checkServer(header *pair, index uint8) bool {
	r.indexes.server = index
	return true
}
func (r *hResponse_) checkSetCookie(header *pair, index uint8) bool {
	return r.parseSetCookie(header.value)
}

func (r *hResponse_) unsafeDate() []byte { // used by proxies
	if r.indexes.date == 0 {
		return nil
	}
	return r.primes[r.indexes.date].valueAt(r.input)
}
func (r *hResponse_) unsafeLastModified() []byte { // used by proxies
	if r.indexes.lastModified == 0 {
		return nil
	}
	return r.primes[r.indexes.lastModified].valueAt(r.input)
}
func (r *hResponse_) unsafeETag() []byte { // used by proxies
	if r.indexes.etag == 0 {
		return nil
	}
	return r.primes[r.indexes.etag].valueAt(r.input)
}

func (r *hResponse_) parseSetCookie(setCookieString text) bool {
	// set-cookie-header = "Set-Cookie:" SP set-cookie-string
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
	var cookie setCookie
	// TODO: parse
	return r.addSetCookie(&cookie)
}
func (r *hResponse_) addSetCookie(cookie *setCookie) bool {
	if len(r.setCookies) == cap(r.setCookies) {
		if cap(r.setCookies) == cap(r.stockSetCookies) {
			setCookies := make([]setCookie, 0, 16)
			r.setCookies = append(setCookies, r.setCookies...)
		} else if cap(r.setCookies) == 16 {
			setCookies := make([]setCookie, 0, 128)
			r.setCookies = append(setCookies, r.setCookies...)
		} else {
			r.headResult = StatusRequestHeaderFieldsTooLarge
			return false
		}
	}
	r.setCookies = append(r.setCookies, *cookie)
	return true
}

func (r *hResponse_) checkHead() bool {
	// Resolve r.keepAlive
	if r.keepAlive == -1 { // no connection header
		switch r.versionCode {
		case Version1_0:
			r.keepAlive = 0 // default is close for HTTP/1.0
		case Version1_1:
			r.keepAlive = 1 // default is keep-alive for HTTP/1.1
		}
	}
	// Resolve r.contentSize
	r.maxContentSize = r.stream.getHolder().(httpClient).MaxContentSize()
	if r.transferChunked { // there is a transfer-encoding: chunked
		if r.versionCode == Version1_0 {
			r.headResult, r.headReason = StatusBadRequest, "transfer-encoding is not used in http/1.0"
			return false
		}
		if r.contentSize == -1 { // content-length does not exist
			r.contentSize = -2 // mark as chunked. use -2 to check chunked content from now on
		} else {
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
	} else if r.contentSize > r.maxContentSize {
		r.headResult = StatusContentTooLarge
		return false
	}
	if r.status < StatusOK && r.contentSize != -1 {
		r.headResult, r.headReason = StatusBadRequest, "1xx responses don't allow content"
		return false
	}
	return true
}

func (r *hResponse_) delCriticalHeaders() { // used by proxies
	r.delPrime(r.indexes.server)
	r.delPrime(r.indexes.date)
	r.delPrime(r.indexes.lastModified)
	r.delPrime(r.indexes.etag)
	r.delPrime(r.iContentLength)
}

func (r *hResponse_) HasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified)
	// responses do not include content.
	if r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might
	// be of zero length.
	return r.contentSize >= 0 || r.contentSize == -2
}
func (r *hResponse_) UnsafeContent() []byte {
	return r.unsafeContent()
}

func (r *hResponse_) applyTrailer(trailer *pair) bool {
	r.addTrailer(trailer)
	// TODO: check trailer? Pseudo-header fields MUST NOT appear in a trailer section.
	return true
}

func (r *hResponse_) arrayCopy(p []byte) bool {
	if len(p) > 0 {
		edge := r.arrayEdge + int32(len(p))
		if edge < r.arrayEdge { // overflow
			return false
		}
		if !r._growArray(int32(len(p))) {
			return false
		}
		r.arrayEdge += int32(copy(r.array[r.arrayEdge:], p))
	}
	return true
}

func (r *hResponse_) getSaveContentFilesDir() string {
	return r.stream.getHolder().(httpClient).SaveContentFilesDir() // must ends with '/'
}

// setCookie is a "set-cookie" received from server.
type setCookie struct { // 24 bytes. refers to r.input
	nameFrom     int16 // foo
	valueFrom    int16 // bar
	valueEdge    int16
	expireFrom   int16 // Expires=Wed, 09 Jun 2021 10:18:14 GMT
	maxAgeFrom   int16 // Max-Age=123
	domainFrom   int16 // Domain=example.com
	pathFrom     int16 // Path=/abc
	sameSiteFrom int16 // SameSite=Lax
	secure       bool  // Secure
	httpOnly     bool  // HttpOnly
	nameSize     uint8
	expireSize   uint8
	maxAgeSize   uint8
	domainSize   uint8
	pathSize     uint8
	sameSiteSize uint8
}

// socket is the client-side HTTP websocket and the interface for *H[1-3]Socket.
type socket interface {
}

// socket_ is the mixin for H[1-3]Socket.
type hSocket_ struct {
	// Assocs
	shell socket // the concrete hSocket
	// Stream states (zeros)
}

func (s *hSocket_) onUse() {
}
func (s *hSocket_) onEnd() {
}
