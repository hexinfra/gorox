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
	sendTimeout    time.Duration // timeout to send the whole request
	recvTimeout    time.Duration // timeout to recv the whole response content
}

func (h *httpClient_) onCreate() {
}

func (h *httpClient_) onConfigure(shell Component, clientType string) {
	h.streamKeeper_.onConfigure(shell, 1000)
	h.contentSaver_.onConfigure(shell, TempDir()+"/http/"+clientType+"/"+shell.Name())
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
	f.httpClient_.onConfigure(shell, "outgate")
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
	b.httpClient_.onConfigure(shell, "backend")
	b.loadBalancer_.onConfigure(shell)
}
func (b *httpBackend_[N]) onPrepare(shell Component, numNodes int) {
	b.backend_.onPrepare()
	b.httpClient_.onPrepare(shell)
	b.loadBalancer_.onPrepare(numNodes)
}

// httpNode_ is the mixin for http[1-3]Node.
type httpNode_ struct {
	// Mixins
	node_
	// States
}

func (n *httpNode_) init(id int32) {
	n.node_.init(id)
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

func (s *hStream_) onUse() {
	s.stream_.onUse()
}
func (s *hStream_) onEnd() {
	s.stream_.onEnd()
}

func (s *hStream_) callTCPTun() { // CONNECT method
	// TODO
}
func (s *hStream_) callUDPTun() { // upgrade: connect-udp
	// TODO
}
func (s *hStream_) callSocket() { // upgrade: websocket
	// TODO
}

// hRequest is the client-side HTTP request and the interface for *H[1-3]Request.
type hRequest interface {
	setMethodURI(method []byte, uri []byte, hasContent bool) bool
	setAuthority(hostname []byte, colonPort []byte) bool // used by proxies
	copyCookies(req Request) bool                        // HTTP 1/2/3 have different requirements on "cookie" header
}

// hRequest_ is the mixin for H[1-3]Request.
type hRequest_ struct { // outgoing. needs building
	// Mixins
	httpOut_
	// Assocs
	response hResponse // the corresponding response
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non-zeros)
	unixTimes struct {
		ifModifiedSince   int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
		ifUnmodifiedSince int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
	}
	// Stream states (zeros)
	hRequest0_ // all values must be zero by default in this struct!
}
type hRequest0_ struct { // for fast reset, entirely
	indexes struct {
		host              uint8
		ifModifiedSince   uint8
		ifRange           uint8
		ifUnmodifiedSince uint8
	}
}

func (r *hRequest_) onUse(versionCode uint8) { // for non-zeros
	r.httpOut_.onUse(versionCode, true) // asRequest = true
	r.unixTimes.ifModifiedSince = -1    // not set
	r.unixTimes.ifUnmodifiedSince = -1  // not set
}
func (r *hRequest_) onEnd() { // for zeros
	r.hRequest0_ = hRequest0_{}
	r.httpOut_.onEnd()
}

func (r *hRequest_) Response() hResponse { return r.response }

func (r *hRequest_) setScheme(scheme []byte) bool { // HTTP/2 and HTTP/3 only
	// TODO: copy `:scheme $scheme` to r.fields
	return false
}
func (r *hRequest_) control() []byte { return r.fields[0:r.controlEdge] } // TODO: maybe we need a struct type to represent pseudo headers?

func (r *hRequest_) SetIfModifiedSince(since int64) bool {
	return r._setUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince, since)
}
func (r *hRequest_) SetIfUnmodifiedSince(since int64) bool {
	return r._setUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince, since)
}

func (r *hRequest_) send() error { return r.shell.sendChain(r.content) }

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
	var chain Chain
	chain.PushTail(chunk)
	defer chain.free()

	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	return r.shell.pushChain(chain)
}
func (r *hRequest_) endUnsized() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	return r.shell.finalizeUnsized()
}

func (r *hRequest_) copyHead(req Request, hostname []byte, colonPort []byte, viaName []byte) bool { // used by proxies
	req.delHopHeaders()

	// copy control (:method, :path, :authority, :scheme)
	var uri []byte
	if req.IsAsteriskOptions() { // OPTIONS *
		// RFC 9112 (3.2.4):
		// If a proxy receives an OPTIONS request with an absolute-form of request-target in which the URI has an empty path and no query component,
		// then the last proxy on the request chain MUST send a request-target of "*" when it forwards the request to the indicated origin server.
		uri = bytesAsterisk
	} else {
		uri = req.UnsafeURI()
	}
	if !r.shell.(hRequest).setMethodURI(req.UnsafeMethod(), uri, req.HasContent()) {
		return false
	}
	if req.IsAbsoluteForm() || len(hostname) != 0 || len(colonPort) != 0 { // TODO: HTTP/2 and HTTP/3?
		req.unsetHost()
		if req.IsAbsoluteForm() {
			if !r.shell.addHeader(bytesHost, req.UnsafeAuthority()) {
				return false
			}
		} else { // custom authority (hostname or colonPort)
			if len(hostname) == 0 {
				hostname = req.UnsafeHostname()
			}
			if len(colonPort) == 0 {
				colonPort = req.UnsafeColonPort()
			}
			if !r.shell.(hRequest).setAuthority(hostname, colonPort) {
				return false
			}
		}
	}
	if r.versionCode >= Version2 { // we have no way to set scheme unless we use absolute-form for HTTP/1.1, which is a risk that many servers don't support it.
		var scheme []byte
		if r.stream.keeper().TLSMode() {
			scheme = bytesSchemeHTTPS
		} else {
			scheme = bytesSchemeHTTP
		}
		if !r.setScheme(scheme) {
			return false
		}
	}

	// copy selective forbidden headers (including cookie) from req
	if req.HasCookies() && !r.shell.(hRequest).copyCookies(req) {
		return false
	}
	// TODO: An HTTP-to-HTTP gateway MUST send an appropriate Via header field in each inbound request message and MAY send a Via header field in forwarded response messages.
	// r.addHeader(viaName)

	// copy remaining headers from req
	if !req.forHeaders(func(header *pair, name []byte, value []byte) bool {
		return r.shell.insertHeader(header.hash, name, value)
	}) {
		return false
	}

	return true
}

var ( // perfect hash table for request critical headers
	hRequestCriticalHeaderNames = []byte("connection content-length content-type cookie date host if-modified-since if-range if-unmodified-since transfer-encoding upgrade via")
	hRequestCriticalHeaderTable = [12]struct {
		hash uint16
		from uint8
		edge uint8
		fAdd func(*hRequest_, []byte) (ok bool)
		fDel func(*hRequest_) (deleted bool)
	}{
		0:  {hashContentLength, 11, 25, nil, nil}, // forbidden
		1:  {hashConnection, 0, 10, nil, nil},     // forbidden
		2:  {hashIfRange, 74, 82, (*hRequest_).appendIfRange, (*hRequest_).deleteIfRange},
		3:  {hashUpgrade, 121, 128, nil, nil}, // forbidden
		4:  {hashIfModifiedSince, 56, 73, (*hRequest_).appendIfModifiedSince, (*hRequest_).deleteIfModifiedSince},
		5:  {hashIfUnmodifiedSince, 83, 102, (*hRequest_).appendIfUnmodifiedSince, (*hRequest_).deleteIfUnmodifiedSince},
		6:  {hashHost, 51, 55, (*hRequest_).appendHost, (*hRequest_).deleteHost},
		7:  {hashTransferEncoding, 103, 120, nil, nil}, // forbidden
		8:  {hashContentType, 26, 38, (*hRequest_).appendContentType, (*hRequest_).deleteContentType},
		9:  {hashCookie, 39, 45, nil, nil}, // forbidden
		10: {hashDate, 46, 50, (*hRequest_).appendDate, (*hRequest_).deleteDate},
		11: {hashVia, 129, 132, nil, nil}, // forbidden
	}
	hRequestCriticalHeaderFind = func(hash uint16) int { return (645048 / int(hash)) % 12 }
)

func (r *hRequest_) insertHeader(hash uint16, name []byte, value []byte) bool {
	h := &hRequestCriticalHeaderTable[hRequestCriticalHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(hRequestCriticalHeaderNames[h.from:h.edge], name) {
		if h.fAdd == nil { // mainly because this header is forbidden
			return true // pretend to be successful
		}
		return h.fAdd(r, value)
	}
	return r.shell.addHeader(name, value)
}
func (r *hRequest_) appendHost(host []byte) (ok bool) {
	return r._appendSingleton(&r.indexes.host, bytesHost, host)
}
func (r *hRequest_) appendIfModifiedSince(since []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince, bytesIfModifiedSince, since)
}
func (r *hRequest_) appendIfRange(ifRange []byte) (ok bool) {
	return r._appendSingleton(&r.indexes.ifRange, bytesIfRange, ifRange)
}
func (r *hRequest_) appendIfUnmodifiedSince(since []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince, bytesIfUnmodifiedSince, since)
}

func (r *hRequest_) removeHeader(hash uint16, name []byte) bool {
	h := &hRequestCriticalHeaderTable[hRequestCriticalHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(hRequestCriticalHeaderNames[h.from:h.edge], name) {
		if h.fDel == nil { // mainly because this header is forbidden
			return true // pretend to be successful
		}
		return h.fDel(r)
	}
	return r.shell.delHeader(name)
}
func (r *hRequest_) deleteHost() (deleted bool) {
	return r._deleteSingleton(&r.indexes.host)
}
func (r *hRequest_) deleteIfModifiedSince() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince)
}
func (r *hRequest_) deleteIfRange() (deleted bool) {
	return r._deleteSingleton(&r.indexes.ifRange)
}
func (r *hRequest_) deleteIfUnmodifiedSince() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince)
}

// upload is a file to be uploaded.
type upload struct {
	// TODO
}

// hResponse is the client-side HTTP response and interface for *H[1-3]Response.
type hResponse interface {
	Status() int16
	delHopHeaders()
	forHeaders(fn func(header *pair, name []byte, value []byte) bool) bool
	delHopTrailers()
	forTrailers(fn func(header *pair, name []byte, value []byte) bool) bool
}

// hResponse_ is the mixin for H[1-3]Response.
type hResponse_ struct { // incoming. needs parsing
	// Mixins
	httpIn_
	// Stream states (buffers)
	stockCookies [8]cookie // for r.cookies
	// Stream states (controlled)
	cookie cookie // to overcome the limitation of Go's escape analysis when receiving setCookies
	// Stream states (non-zeros)
	cookies []cookie // hold setCookies->r.input. [<r.stockCookies>/(make=32/128)]
	// Stream states (zeros)
	hResponse0_ // all values must be zero by default in this struct!
}
type hResponse0_ struct { // for fast reset, entirely
	status    int16    // 200, 302, 404, ...
	unixTimes struct { // parsed unix times
		lastModified int64 // parsed unix time of last-modified
		expires      int64 // parsed unix time of expires
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
	indexes struct { // indexes of some selected headers, for fast accessing
		server       uint8 // server header ->r.input
		lastModified uint8 // last-modified header ->r.input
		expires      uint8 // expires header ->r.input
		etag         uint8 // etag header ->r.input
		acceptRanges uint8 // accept-ranges header ->r.input
		location     uint8 // location header ->r.input
	}
}

func (r *hResponse_) onUse(versionCode uint8) { // for non-zeros
	r.httpIn_.onUse(versionCode, true) // asResponse = true

	r.cookies = r.stockCookies[0:0:cap(r.stockCookies)] // use append()
}
func (r *hResponse_) onEnd() { // for zeros
	if cap(r.cookies) != cap(r.stockCookies) {
		// TODO: put?
		r.cookies = nil
	}
	r.hResponse0_ = hResponse0_{}

	r.httpIn_.onEnd()
}

func (r *hResponse_) Status() int16 { return r.status }

func (r *hResponse_) adoptHeader(header *pair) bool {
	headerName := header.nameAt(r.input)
	if h := &hResponseSingletonHeaderTable[hResponseSingletonHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(hResponseSingletonHeaderNames[h.from:h.edge], headerName) {
		header.setSingleton()
		if !r._setFieldData(header, h.quote, h.empty, h.para) {
			// r.headResult is set.
			return false
		}
		if h.check != nil && !h.check(r, header, r.headers.edge-1) {
			// r.headResult is set.
			return false
		}
	} else { // all other headers are treated as multiple headers
		from := r.headers.edge + 1 // excluding main header
		if !r._addSubFields(header, h.quote, h.empty, h.para, r.input, r.addHeader) {
			// r.headResult is set.
			return false
		}
		if h := &hResponseImportantHeaderTable[hResponseImportantHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(hResponseImportantHeaderNames[h.from:h.edge], headerName) {
			if h.check != nil && !h.check(r, from, r.headers.edge) {
				// r.headResult is set.
				return false
			}
		}
	}
	return true
}

var ( // perfect hash table for response singleton headers
	hResponseSingletonHeaderNames = []byte("age content-length content-range content-type date etag expires last-modified location retry-after server set-cookie")
	hResponseSingletonHeaderTable = [12]struct {
		hash  uint16
		from  uint8
		edge  uint8
		quote bool // allow data quote or not
		empty bool // allow empty data or not
		para  bool // allow parameters or not
		check func(*hResponse_, *pair, uint8) bool
	}{
		0:  {hashDate, 46, 50, false, false, false, (*hResponse_).checkDate},
		1:  {hashContentLength, 4, 18, false, false, false, (*hResponse_).checkContentLength},
		2:  {hashAge, 0, 3, false, false, false, (*hResponse_).checkAge},
		3:  {hashSetCookie, 106, 116, false, false, false, (*hResponse_).checkSetCookie}, // `a=b; Path=/; HttpsOnly` is not parameters
		4:  {hashLastModified, 64, 77, false, false, false, (*hResponse_).checkLastModified},
		5:  {hashLocation, 78, 86, false, false, false, (*hResponse_).checkLocation},
		6:  {hashExpires, 56, 63, false, false, false, (*hResponse_).checkExpires},
		7:  {hashContentRange, 19, 32, false, false, false, (*hResponse_).checkContentRange},
		8:  {hashETag, 51, 55, false, false, false, (*hResponse_).checkETag},
		9:  {hashServer, 99, 105, false, false, false, (*hResponse_).checkServer},
		10: {hashContentType, 33, 45, false, false, true, (*hResponse_).checkContentType},
		11: {hashRetryAfter, 87, 98, false, false, false, (*hResponse_).checkRetryAfter},
	}
	hResponseSingletonHeaderFind = func(hash uint16) int { return (889344 / int(hash)) % 12 }
)

func (r *hResponse_) checkAge(header *pair, index uint8) bool { // Age = delta-seconds
	// TODO
	return true
}
func (r *hResponse_) checkETag(header *pair, index uint8) bool { // ETag = entity-tag
	r.indexes.etag = index
	return true
}
func (r *hResponse_) checkExpires(header *pair, index uint8) bool { // Expires = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.expires, &r.unixTimes.expires)
}
func (r *hResponse_) checkLastModified(header *pair, index uint8) bool { // Last-Modified = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.lastModified, &r.unixTimes.lastModified)
}
func (r *hResponse_) checkLocation(header *pair, index uint8) bool { // Location = URI-reference
	r.indexes.location = index
	return true
}
func (r *hResponse_) checkRetryAfter(header *pair, index uint8) bool { // Retry-After = HTTP-date / delay-seconds
	return true
}
func (r *hResponse_) checkServer(header *pair, index uint8) bool { // Server = product *( RWS ( product / comment ) )
	r.indexes.server = index
	return true
}
func (r *hResponse_) checkSetCookie(header *pair, index uint8) bool { // Set-Cookie = set-cookie-string
	if !r.parseSetCookie(header.value) {
		r.headResult, r.failReason = StatusBadRequest, "bad set-cookie"
		return false
	}
	if len(r.cookies) == cap(r.cookies) {
		if cap(r.cookies) == cap(r.stockCookies) {
			cookies := make([]cookie, 0, 16)
			r.cookies = append(cookies, r.cookies...)
		} else if cap(r.cookies) == 16 {
			cookies := make([]cookie, 0, 128)
			r.cookies = append(cookies, r.cookies...)
		} else {
			r.headResult = StatusRequestHeaderFieldsTooLarge
			return false
		}
	}
	r.cookies = append(r.cookies, r.cookie)
	return true
}

var ( // perfect hash table for response important headers
	hResponseImportantHeaderNames = []byte("accept-encoding accept-ranges allow alt-svc cache-control cache-status cdn-cache-control connection content-encoding content-language proxy-authenticate trailer transfer-encoding upgrade vary via www-authenticate")
	hResponseImportantHeaderTable = [17]struct {
		hash  uint16
		from  uint8
		edge  uint8
		quote bool // allow data quote or not
		empty bool // allow empty data or not
		para  bool // allow parameters or not
		check func(*hResponse_, uint8, uint8) bool
	}{
		0:  {hashAcceptRanges, 16, 29, false, false, false, (*hResponse_).checkAcceptRanges},
		1:  {hashVia, 192, 195, false, false, false, (*hResponse_).checkVia},
		2:  {hashWWWAuthenticate, 196, 212, false, false, false, (*hResponse_).checkWWWAuthenticate},
		3:  {hashConnection, 89, 99, false, false, false, (*hResponse_).checkConnection},
		4:  {hashContentEncoding, 100, 116, false, false, false, (*hResponse_).checkContentEncoding},
		5:  {hashAllow, 30, 35, false, true, false, (*hResponse_).checkAllow},
		6:  {hashTransferEncoding, 161, 178, false, false, false, (*hResponse_).checkTransferEncoding}, // deliberately false
		7:  {hashTrailer, 153, 160, false, false, false, (*hResponse_).checkTrailer},
		8:  {hashVary, 187, 191, false, false, false, (*hResponse_).checkVary},
		9:  {hashUpgrade, 179, 186, false, false, false, (*hResponse_).checkUpgrade},
		10: {hashProxyAuthenticate, 134, 152, false, false, false, (*hResponse_).checkProxyAuthenticate},
		11: {hashCacheControl, 44, 57, false, false, false, (*hResponse_).checkCacheControl},
		12: {hashAltSvc, 36, 43, false, false, true, (*hResponse_).checkAltSvc},
		13: {hashCDNCacheControl, 71, 88, false, false, false, (*hResponse_).checkCDNCacheControl},
		14: {hashCacheStatus, 58, 70, false, false, true, (*hResponse_).checkCacheStatus},
		15: {hashAcceptEncoding, 0, 15, false, true, true, (*hResponse_).checkAcceptEncoding},
		16: {hashContentLanguage, 117, 133, false, false, false, (*hResponse_).checkContentLanguage},
	}
	hResponseImportantHeaderFind = func(hash uint16) int { return (72189325 / int(hash)) % 17 }
)

func (r *hResponse_) checkAcceptRanges(from uint8, edge uint8) bool { // Accept-Ranges = 1#range-unit
	if from == edge {
		r.headResult, r.failReason = StatusBadRequest, "accept-ranges = 1#range-unit"
		return false
	}
	// TODO
	return true
}
func (r *hResponse_) checkAllow(from uint8, edge uint8) bool { // Allow = #method
	// TODO
	return true
}
func (r *hResponse_) checkAltSvc(from uint8, edge uint8) bool { // Alt-Svc = clear / 1#alt-value
	if from == edge {
		r.headResult, r.failReason = StatusBadRequest, "alt-svc = clear / 1#alt-value"
		return false
	}
	// TODO
	return true
}
func (r *hResponse_) checkCacheControl(from uint8, edge uint8) bool { // Cache-Control = #cache-directive
	// cache-directive = token [ "=" ( token / quoted-string ) ]
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *hResponse_) checkCacheStatus(from uint8, edge uint8) bool { // ?
	// TODO
	return true
}
func (r *hResponse_) checkCDNCacheControl(from uint8, edge uint8) bool { // ?
	// TODO
	return true
}
func (r *hResponse_) checkProxyAuthenticate(from uint8, edge uint8) bool { // Proxy-Authenticate = #challenge
	// TODO
	return true
}
func (r *hResponse_) checkTransferEncoding(from uint8, edge uint8) bool { // Transfer-Encoding = #transfer-coding
	if r.status < StatusOK || r.status == StatusNoContent {
		r.headResult, r.failReason = StatusBadRequest, "transfer-encoding is not allowed in 1xx and 204 responses"
		return false
	}
	if r.status == StatusNotModified {
		// TODO
	}
	return r.httpIn_.checkTransferEncoding(from, edge)
}
func (r *hResponse_) checkUpgrade(from uint8, edge uint8) bool { // Upgrade = #protocol
	// TODO: tcptun, udptun, socket?
	r.headResult, r.failReason = StatusBadRequest, "upgrade is not supported in normal mode"
	return false
}
func (r *hResponse_) checkVary(from uint8, edge uint8) bool { // Vary = #( "*" / field-name )
	// TODO
	return true
}
func (r *hResponse_) checkWWWAuthenticate(from uint8, edge uint8) bool { // WWW-Authenticate = #challenge
	// TODO
	return true
}

func (r *hResponse_) parseSetCookie(setCookieString text) bool { // set-cookie-string = cookie-pair *( ";" SP cookie-av )
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
	cookie := &r.cookie
	cookie.zero()
	// TODO
	return true
}

func (r *hResponse_) checkHead() bool {
	// Basic checks against versions
	switch r.versionCode {
	case Version1_0: // we don't support HTTP/1.0 in client side
		BugExitln("HTTP/1.0 must be denied prior")
	case Version1_1:
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

	if r.status < StatusOK && r.contentSize != -1 {
		r.headResult, r.failReason = StatusBadRequest, "content is not allowed in 1xx responses"
		return false
	}
	r.maxContentSize = r.stream.keeper().(httpClient).MaxContentSize()
	if r.contentSize > r.maxContentSize {
		r.headResult = StatusContentTooLarge
		return false
	}
	return true
}

func (r *hResponse_) unsafeDate() []byte {
	if r.iDate == 0 {
		return nil
	}
	return r.primes[r.iDate].valueAt(r.input)
}
func (r *hResponse_) unsafeLastModified() []byte {
	if r.indexes.lastModified == 0 {
		return nil
	}
	return r.primes[r.indexes.lastModified].valueAt(r.input)
}

func (r *hResponse_) addCookie(cookie *cookie) bool {
	// TODO
	return true
}
func (r *hResponse_) HasCookies() bool {
	// TODO
	return false
}
func (r *hResponse_) C(name string) string {
	// TODO
	return ""
}
func (r *hResponse_) Cookie(name string) (value string, ok bool) {
	// TODO
	return
}
func (r *hResponse_) UnsafeCookie(name []byte) (value []byte, ok bool) {
	// TODO
	return
}
func (r *hResponse_) HasCookie(name string) bool {
	// TODO
	return false
}
func (r *hResponse_) forCookies(fn func(cookie *pair, name []byte, value []byte) bool) bool {
	// TODO
	return false
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

func (r *hResponse_) adoptTrailer(trailer *pair) bool {
	// TODO: Pseudo-header fields MUST NOT appear in a trailer section.
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

// cookie is a "set-cookie" received from server.
type cookie struct { // 24 bytes. refers to r.input
	hash         uint16 // hash of name
	nameFrom     int16  // foo
	valueFrom    int16  // bar
	valueEdge    int16  // edge of value
	expiresFrom  int16  // Expires=Wed, 09 Jun 2021 10:18:14 GMT (fixed value length=29)
	maxAgeFrom   int16  // Max-Age=123
	domainFrom   int16  // Domain=example.com
	pathFrom     int16  // Path=/abc
	sameSiteFrom int16  // SameSite=Lax|Strict|None
	nameSize     uint8  // <= 255
	maxAgeSize   uint8  // <= 255
	domainSize   uint8  // <= 255
	pathSize     uint8  // <= 255
	sameSiteSize uint8  // <= 255
	flags        uint8  // secure(1), httpOnly(1), reserved(6)
}

func (c *cookie) zero() { *c = cookie{} }

func (c *cookie) nameAt(t []byte) []byte {
	return t[c.nameFrom : c.nameFrom+int16(c.nameSize)]
}
func (c *cookie) valueAt(t []byte) []byte {
	return t[c.valueFrom:c.valueEdge]
}
func (c *cookie) expiresAt(t []byte) []byte {
	return t[c.expiresFrom : c.expiresFrom+29]
}
func (c *cookie) maxAgeAt(t []byte) []byte {
	return t[c.maxAgeFrom : c.maxAgeFrom+int16(c.maxAgeSize)]
}
func (c *cookie) domainAt(t []byte) []byte {
	return t[c.domainFrom : c.domainFrom+int16(c.domainSize)]
}
func (c *cookie) pathAt(t []byte) []byte {
	return t[c.pathFrom : c.pathFrom+int16(c.pathSize)]
}
func (c *cookie) sameSiteAt(t []byte) []byte {
	return t[c.sameSiteFrom : c.sameSiteFrom+int16(c.sameSiteSize)]
}
func (c *cookie) secure() bool   { return c.flags&0b10000000 > 0 }
func (c *cookie) httpOnly() bool { return c.flags&0b01000000 > 0 }

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
