// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
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
	streamKeeper
	contentSaver
	MaxContentSize() int64
	SendTimeout() time.Duration
	RecvTimeout() time.Duration
}

// httpClient_ is a mixin for httpOutgate_ and httpBackend_.
type httpClient_ struct {
	// Mixins
	streamKeeper_
	contentSaver_ // so responses can save their large contents in local file system.
	// States
	maxContentSize int64
	sendTimeout    time.Duration
	recvTimeout    time.Duration
}

func (h *httpClient_) onCreate() {
}

func (h *httpClient_) onConfigure(shell Component, clientName string) {
	h.streamKeeper_.onConfigure(shell, 1000)
	h.contentSaver_.onConfigure(shell, TempDir()+"/"+clientName+"/"+shell.Name())
	// maxContentSize
	shell.ConfigureInt64("maxContentSize", &h.maxContentSize, func(value int64) bool { return value > 0 }, _1T)
	// sendTimeout
	shell.ConfigureDuration("sendTimeout", &h.sendTimeout, func(value time.Duration) bool { return value > 0 }, 60*time.Second)
	// recvTimeout
	shell.ConfigureDuration("recvTimeout", &h.recvTimeout, func(value time.Duration) bool { return value > 0 }, 60*time.Second)
}
func (h *httpClient_) onPrepare(shell Component) {
	h.streamKeeper_.onPrepare(shell)
	h.contentSaver_.onPrepare(shell, 0755)
}

func (h *httpClient_) MaxContentSize() int64      { return h.maxContentSize }
func (h *httpClient_) SendTimeout() time.Duration { return h.sendTimeout }
func (h *httpClient_) RecvTimeout() time.Duration { return h.recvTimeout }

// httpOutgate_ is the mixin for HTTP[1-3]Outgate.
type httpOutgate_ struct {
	// Mixins
	outgate_
	httpClient_
	// States
}

func (f *httpOutgate_) onCreate(name string, stage *Stage) {
	f.outgate_.onCreate(name, stage)
	f.httpClient_.onCreate()
}

func (f *httpOutgate_) onConfigure(shell Component) {
	f.outgate_.onConfigure()
	f.httpClient_.onConfigure(shell, "outgates")
}
func (f *httpOutgate_) onPrepare(shell Component) {
	f.outgate_.onPrepare()
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
	makeTempName(p []byte, stamp int64) (from int, edge int) // small enough to be placed in smallBuffer() of stream
	isBroken() bool
	markBroken()
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

func (c *hConn_) makeTempName(p []byte, stamp int64) (from int, edge int) {
	return makeTempName(p, int64(c.client.Stage().ID()), c.id, stamp, c.counter.Add(1))
}

func (c *hConn_) reachLimit() bool {
	return c.usedStreams.Add(1) > c.getClient().MaxStreamsPerConn()
}

func (c *hConn_) isBroken() bool { return c.broken.Load() }
func (c *hConn_) markBroken()    { c.broken.Store(true) }

// hStream_ is the mixin for H[1-3]Stream.
type hStream_ struct {
	// Mixins
	stream_
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *hStream_) callTCPTun() { // CONNECT method
	// TODO
}
func (s *hStream_) callUDPTun() { // upgrade: connect-udp
	// TODO
}
func (s *hStream_) callSocket() { // upgrade: wegsocket
	// TODO
}

// request is the client-side HTTP request and the interface for *H[1-3]Request.
type request interface {
	Response() response
	SetSendTimeout(timeout time.Duration) // to defend against bad server
	setControl(method []byte, uri []byte, hasContent bool) bool
	setAuthority(hostname []byte, colonPort []byte) bool // used by proxies
	addHeader(name []byte, value []byte) bool
	copyCookies(req Request) bool
}

// hRequest_ is the mixin for H[1-3]Request.
type hRequest_ struct { // outgoing. needs building
	// Mixins
	httpOut_
	// Assocs
	response response // the corresponding response
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	ifModifiedSince   int64 // ...
	ifUnmodifiedSince int64 // ...
	// Stream states (zeros)
	hRequest0_ // all values must be zero by default in this struct!
}
type hRequest0_ struct { // for fast reset, entirely
	oHost              uint8 // ...
	oIfModifiedSince   uint8
	oIfRange           uint8
	oIfUnmodifiedSince uint8
}

func (r *hRequest_) onUse() { // for non-zeros
	r.httpOut_.onUse(true)
	r.ifModifiedSince = -1
	r.ifUnmodifiedSince = -1
}
func (r *hRequest_) onEnd() { // for zeros
	r.hRequest0_ = hRequest0_{}
	r.httpOut_.onEnd()
}

func (r *hRequest_) Response() response { return r.response }

func (r *hRequest_) control() []byte { return r.fields[0:r.controlEdge] }

func (r *hRequest_) SetIfModifiedSince(since int64) bool {
	// TODO
	return false
}
func (r *hRequest_) SetIfUnmodifiedSince(since int64) bool {
	// TODO
	return false
}

var ( // perfect hash table for request crucial headers
	hRequestCrucialHeaderNames = []byte("connection content-length content-type cookie date host if-modified-since if-range if-unmodified-since transfer-encoding upgrade")
	hRequestCrucialHeaderTable = [11]struct {
		hash uint16
		from uint8
		edge uint8
		fAdd func(*hRequest_, []byte) (ok bool)
		fDel func(*hRequest_) (deleted bool)
	}{
		0:  {httpHashDate, 46, 50, (*hRequest_).appendDate, (*hRequest_).removeDate},
		1:  {httpHashIfRange, 74, 82, (*hRequest_).appendIfRange, (*hRequest_).removeIfRange},
		2:  {httpHashIfUnmodifiedSince, 83, 102, (*hRequest_).appendIfUnmodifiedSince, (*hRequest_).removeIfUnmodifiedSince},
		3:  {httpHashIfModifiedSince, 56, 73, (*hRequest_).appendIfModifiedSince, (*hRequest_).removeIfModifiedSince},
		4:  {httpHashTransferEncoding, 103, 120, nil, nil},
		5:  {httpHashHost, 51, 55, (*hRequest_).appendHost, (*hRequest_).removeHost},
		6:  {httpHashCookie, 39, 45, nil, nil},
		7:  {httpHashContentLength, 11, 25, nil, nil},
		8:  {httpHashContentType, 26, 38, (*hRequest_).appendContentType, (*hRequest_).removeContentType},
		9:  {httpHashConnection, 0, 10, nil, nil},
		10: {httpHashUpgrade, 121, 128, nil, nil},
	}
	hRequestCrucialHeaderFind = func(hash uint16) int { return (1685160 / int(hash)) % 11 }
)

func (r *hRequest_) appendHeader(hash uint16, name []byte, value []byte) bool {
	h := &hRequestCrucialHeaderTable[hRequestCrucialHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(hRequestCrucialHeaderNames[h.from:h.edge], name) {
		if h.fAdd == nil {
			return true // pretend to be successful
		}
		return h.fAdd(r, value)
	}
	return r.shell.addHeader(name, value)
}
func (r *hRequest_) removeHeader(hash uint16, name []byte) bool {
	h := &hRequestCrucialHeaderTable[hRequestCrucialHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(hRequestCrucialHeaderNames[h.from:h.edge], name) {
		if h.fDel == nil {
			return true // pretend to be successful
		}
		return h.fDel(r)
	}
	return r.shell.delHeader(name)
}

func (r *hRequest_) appendHost(host []byte) (ok bool) {
	// TODO
	return r.shell.addHeader(httpBytesHost, host)
}
func (r *hRequest_) removeHost() (deleted bool) {
	// TODO
	return true
}
func (r *hRequest_) appendIfModifiedSince(since []byte) (ok bool) {
	// TODO
	return true
}
func (r *hRequest_) removeIfModifiedSince() (deleted bool) {
	// TODO
	return true
}
func (r *hRequest_) appendIfRange(ifRange []byte) (ok bool) {
	// TODO
	return true
}
func (r *hRequest_) removeIfRange() (deleted bool) {
	// TODO
	return true
}
func (r *hRequest_) appendIfUnmodifiedSince(since []byte) (ok bool) {
	// TODO
	return true
}
func (r *hRequest_) removeIfUnmodifiedSince() (deleted bool) {
	// TODO
	return true
}

func (r *hRequest_) send() error {
	return r.shell.sendChain(r.content)
}

func (r *hRequest_) checkPush() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	if r.IsSent() {
		return nil
	}
	if r.contentSize != -1 {
		return httpOutMixedContent
	}
	r.markSent()
	r.markUnsized()
	return r.shell.pushHeaders()
}
func (r *hRequest_) push(chunk *Block) error {
	var curChain Chain
	curChain.PushTail(chunk)
	defer curChain.free()

	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	return r.shell.pushChain(curChain)
}

func (r *hRequest_) copyHead(req Request, hostname []byte, colonPort []byte) bool { // used by proxies
	var uri []byte
	if req.IsAsteriskOptions() { // OPTIONS *
		// RFC 9112 (3.2.4):
		// If a proxy receives an OPTIONS request with an absolute-form of request-target in which the URI has an empty path and no query component,
		// then the last proxy on the request chain MUST send a request-target of "*" when it forwards the request to the indicated origin server.
		uri = httpBytesAsterisk
	} else {
		uri = req.UnsafeURI()
	}
	if !r.shell.(request).setControl(req.UnsafeMethod(), uri, req.HasContent()) {
		return false
	}

	req.delHopHeaders()

	// copy crucial headers (including cookie) from req
	if req.HasCookies() && !r.shell.(request).copyCookies(req) {
		return false
	}
	if custom := len(hostname) != 0 || len(colonPort) != 0; custom || req.IsAbsoluteForm() {
		req.unsetHost()
		if custom {
			if len(hostname) == 0 {
				hostname = req.UnsafeHostname()
			}
			if len(colonPort) == 0 {
				colonPort = req.UnsafeColonPort()
			}
			if !r.shell.(request).setAuthority(hostname, colonPort) {
				return false
			}
		} else if !r.shell.addHeader(httpBytesHost, req.UnsafeAuthority()) {
			return false
		}
	}

	// copy remaining headers from req
	if !req.forHeaders(r.shell.appendHeader) {
		return false
	}

	return true
}

func (r *hRequest_) endUnsized() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	return r.shell.finalizeUnsized()
}

// response is the client-side HTTP response and interface for *H[1-3]Response.
type response interface {
	Status() int16
	ContentSize() int64
	SetRecvTimeout(timeout time.Duration) // to defend against bad server
	HasTrailers() bool

	delHopHeaders()
	forHeaders(fn func(hash uint16, name []byte, value []byte) bool) bool
	readContent() (p []byte, err error)
	delHopTrailers()
	forTrailers(fn func(hash uint16, name []byte, value []byte) bool) bool
}

// hResponse_ is the mixin for H[1-3]Response.
type hResponse_ struct { // incoming. needs parsing
	// Mixins
	httpIn_
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
	r.httpIn_.onUse(true) // asResponse = true

	r.setCookies = r.stockSetCookies[0:0:cap(r.stockSetCookies)] // use append()
}
func (r *hResponse_) onEnd() { // for zeros
	if cap(r.setCookies) != cap(r.stockSetCookies) {
		// put?
		r.setCookies = nil
	}
	r.hResponse0_ = hResponse0_{}

	r.httpIn_.onEnd()
}

func (r *hResponse_) Status() int16 { return r.status }

func (r *hResponse_) applyHeader(header *pair) bool {
	headerName := header.nameAt(r.input)
	if h := &hResponseMultipleHeaderTable[hResponseMultipleHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(hResponseMultipleHeaderNames[h.from:h.edge], headerName) {
		if header.value.isEmpty() && h.must {
			r.headResult, r.headReason = StatusBadRequest, "empty value detected for field value format 1#(value)"
			return false
		}
		from := r.headers.edge + 1 // excluding original header. overflow doesn't matter
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
	hResponseMultipleHeaderNames = []byte("accept-encoding accept-ranges allow cache-control connection content-encoding content-language proxy-authenticate trailer transfer-encoding upgrade vary via www-authenticate") // alt-svc?
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
	return r.httpIn_.checkTransferEncoding(from, edge)
}
func (r *hResponse_) checkUpgrade(from uint8, edge uint8) bool {
	r.headResult, r.headReason = StatusBadRequest, "upgrade is not supported in normal mode"
	return false
}

var ( // perfect hash table for response critical headers
	hResponseCriticalHeaderNames = []byte("content-length content-range content-type date etag expires last-modified location server set-cookie") // age?
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

func (r *hResponse_) checkAge(header *pair, index uint8) bool {
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

func (r *hResponse_) unsafeDate() []byte {
	if r.indexes.date == 0 {
		return nil
	}
	return r.primes[r.indexes.date].valueAt(r.input)
}
func (r *hResponse_) unsafeLastModified() []byte {
	if r.indexes.lastModified == 0 {
		return nil
	}
	return r.primes[r.indexes.lastModified].valueAt(r.input)
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
	// TODO: parse to cookie
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
	r.setCookies = append(r.setCookies, cookie)
	return true
}
func (r *hResponse_) hasSetCookies() bool { return len(r.setCookies) > 0 }

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
	r.maxContentSize = r.stream.keeper().(httpClient).MaxContentSize()
	if r.transferChunked { // there is a transfer-encoding: chunked
		if r.versionCode == Version1_0 {
			r.headResult, r.headReason = StatusBadRequest, "transfer-encoding is not used in http/1.0"
			return false
		}
		if r.contentSize == -1 { // content-length does not exist
			r.markUnsized()
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
		r.headResult, r.headReason = StatusBadRequest, "content is not allowed in 1xx responses"
		return false
	}
	return true
}

func (r *hResponse_) HasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified)
	// responses do not include content.
	if r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might
	// be of zero length.
	return r.contentSize >= 0 || r.isUnsized()
}
func (r *hResponse_) Content() string       { return string(r.unsafeContent()) }
func (r *hResponse_) UnsafeContent() []byte { return r.unsafeContent() }

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

func (r *hResponse_) saveContentFilesDir() string {
	return r.stream.keeper().(httpClient).SaveContentFilesDir() // must ends with '/'
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
