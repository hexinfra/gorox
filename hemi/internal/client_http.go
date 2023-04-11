// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General HTTP client implementation.

package internal

import (
	"bytes"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"sync/atomic"
	"time"
)

// httpClient is the interface for http outgates and http backends.
type httpClient interface {
	client
	streamHolder
	contentSaver
	MaxContentSize() int64
	SendTimeout() time.Duration
	RecvTimeout() time.Duration
}

// httpClient_ is a mixin for httpOutgate_ and httpBackend_.
type httpClient_ struct {
	// Mixins
	keeper_
	streamHolder_
	contentSaver_ // so responses can save their large contents in local file system.
	// States
}

func (h *httpClient_) onCreate() {
}

func (h *httpClient_) onConfigure(shell Component, clientType string) {
	h.streamHolder_.onConfigure(shell, 1000)
	h.contentSaver_.onConfigure(shell, TempDir()+"/http/"+clientType+"/"+shell.Name())
	// maxContentSize
	shell.ConfigureInt64("maxContentSize", &h.maxContentSize, func(value int64) bool { return value > 0 }, _1T)
	// sendTimeout
	shell.ConfigureDuration("sendTimeout", &h.sendTimeout, func(value time.Duration) bool { return value > 0 }, 60*time.Second)
	// recvTimeout
	shell.ConfigureDuration("recvTimeout", &h.recvTimeout, func(value time.Duration) bool { return value > 0 }, 60*time.Second)
}
func (h *httpClient_) onPrepare(shell Component) {
	h.streamHolder_.onPrepare(shell)
	h.contentSaver_.onPrepare(shell, 0755)
}

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
	makeTempName(p []byte, unixTime int64) (from int, edge int) // small enough to be placed in buffer256() of stream
	isBroken() bool
	markBroken()
}

// hConn_ is the mixin for H[1-3]Conn.
type hConn_ struct {
	// Mixins
	conn_
	// Conn states (stocks)
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

func (c *hConn_) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return makeTempName(p, int64(c.client.Stage().ID()), c.id, unixTime, c.counter.Add(1))
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
	// Stream states (stocks)
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

func (s *hStream_) startSocket() { // upgrade: websocket
	// TODO
}
func (s *hStream_) startTCPTun() { // CONNECT method
	// TODO
}
func (s *hStream_) startUDPTun() { // upgrade: connect-udp
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
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	unixTimes struct {
		ifModifiedSince   int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
		ifUnmodifiedSince int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
	}
	// Stream states (zeros)
	hRequest0 // all values must be zero by default in this struct!
}
type hRequest0 struct { // for fast reset, entirely
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
	r.hRequest0 = hRequest0{}
	r.httpOut_.onEnd()
}

func (r *hRequest_) Response() hResponse { return r.response }

func (r *hRequest_) SetMethodURI(method string, uri string, hasContent bool) bool {
	return r.shell.(hRequest).setMethodURI(risky.ConstBytes(method), risky.ConstBytes(uri), hasContent)
}
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

func (r *hRequest_) send() error { return r.shell.sendChain() }

func (r *hRequest_) _beforeEcho() error {
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
	return r.shell.echoHeaders()
}
func (r *hRequest_) echo() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	r.chain.PushTail(&r.block)
	defer r.chain.free()
	return r.shell.echoChain()
}
func (r *hRequest_) endUnsized() error {
	if r.stream.isBroken() {
		return httpOutWriteBroken
	}
	return r.shell.finalizeUnsized()
}

func (r *hRequest_) copyHeadFrom(req Request, hostname []byte, colonPort []byte, viaName []byte) bool { // used by proxies
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
	if req.IsAbsoluteForm() || len(hostname) != 0 || len(colonPort) != 0 { // TODO: what about HTTP/2 and HTTP/3?
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
	if r.versionCode >= Version2 {
		var scheme []byte
		if r.stream.keeper().TLSMode() {
			scheme = bytesSchemeHTTPS
		} else {
			scheme = bytesSchemeHTTP
		}
		if !r.setScheme(scheme) {
			return false
		}
	} else {
		// we have no way to set scheme in HTTP/1 unless we use absolute-form, which is a risk that many servers may not support it.
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
	hRequestCriticalHeaderTable = [12]struct {
		hash uint16
		name []byte
		fAdd func(*hRequest_, []byte) (ok bool)
		fDel func(*hRequest_) (deleted bool)
	}{ // connection content-length content-type cookie date host if-modified-since if-range if-unmodified-since transfer-encoding upgrade via
		0:  {hashContentLength, bytesContentLength, nil, nil}, // forbidden
		1:  {hashConnection, bytesConnection, nil, nil},       // forbidden
		2:  {hashIfRange, bytesIfRange, (*hRequest_).appendIfRange, (*hRequest_).deleteIfRange},
		3:  {hashUpgrade, bytesUpgrade, nil, nil}, // forbidden
		4:  {hashIfModifiedSince, bytesIfModifiedSince, (*hRequest_).appendIfModifiedSince, (*hRequest_).deleteIfModifiedSince},
		5:  {hashIfUnmodifiedSince, bytesIfUnmodifiedSince, (*hRequest_).appendIfUnmodifiedSince, (*hRequest_).deleteIfUnmodifiedSince},
		6:  {hashHost, bytesHost, (*hRequest_).appendHost, (*hRequest_).deleteHost},
		7:  {hashTransferEncoding, bytesTransferEncoding, nil, nil}, // forbidden
		8:  {hashContentType, bytesContentType, (*hRequest_).appendContentType, (*hRequest_).deleteContentType},
		9:  {hashCookie, bytesCookie, nil, nil}, // forbidden
		10: {hashDate, bytesDate, (*hRequest_).appendDate, (*hRequest_).deleteDate},
		11: {hashVia, bytesVia, nil, nil}, // forbidden
	}
	hRequestCriticalHeaderFind = func(hash uint16) int { return (645048 / int(hash)) % 12 }
)

func (r *hRequest_) insertHeader(hash uint16, name []byte, value []byte) bool {
	h := &hRequestCriticalHeaderTable[hRequestCriticalHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(h.name, name) {
		if h.fAdd == nil { // mainly because this header is forbidden to insert
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
	if h.hash == hash && bytes.Equal(h.name, name) {
		if h.fDel == nil { // mainly because this header is forbidden to remove
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

func (r *hRequest_) copyTailFrom(req Request) bool { // used by proxies
	return req.forTrailers(func(trailer *pair, name []byte, value []byte) bool {
		return r.shell.addTrailer(name, value)
	})
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
	// Stream states (stocks)
	stockCookies [8]cookie // for r.cookies
	// Stream states (controlled)
	cookie cookie // to overcome the limitation of Go's escape analysis when receiving setCookies
	// Stream states (non-zeros)
	cookies []cookie // hold setCookies->r.input. [<r.stockCookies>/(make=32/128)]
	// Stream states (zeros)
	hResponse0 // all values must be zero by default in this struct!
}
type hResponse0 struct { // for fast reset, entirely
	status      int16    // 200, 302, 404, ...
	acceptBytes bool     // accept-ranges: bytes?
	hasAllow    bool     // has allow header?
	age         int32    // age seconds
	indexes     struct { // indexes of some selected singleton headers, for fast accessing
		etag         uint8   // etag header ->r.input
		expires      uint8   // expires header ->r.input
		lastModified uint8   // last-modified header ->r.input
		location     uint8   // location header ->r.input
		server       uint8   // server header ->r.input
		_            [3]byte // padding
	}
	zones struct { // zones of some selected headers, for fast accessing
		allow  zone
		altSvc zone
		vary   zone
		_      [2]byte // padding
	}
	unixTimes struct { // parsed unix times
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

func (r *hResponse_) onUse(versionCode uint8) { // for non-zeros
	r.httpIn_.onUse(versionCode, true) // asResponse = true

	r.cookies = r.stockCookies[0:0:cap(r.stockCookies)] // use append()
}
func (r *hResponse_) onEnd() { // for zeros
	if cap(r.cookies) != cap(r.stockCookies) {
		// TODO: put?
		r.cookies = nil
	}
	r.hResponse0 = hResponse0{}

	r.httpIn_.onEnd()
}

func (r *hResponse_) Status() int16 { return r.status }

func (r *hResponse_) examineHead() bool {
	for i := r.headers.from; i < r.headers.edge; i++ {
		if !r.applyHeader(i) {
			// r.headResult is set.
			return false
		}
	}
	if IsDebug(2) {
		for i := 0; i < len(r.primes); i++ {
			prime := &r.primes[i]
			prime.show(r._placeOf(prime))
		}
		for i := 0; i < len(r.extras); i++ {
			extra := &r.extras[i]
			extra.show(r._placeOf(extra))
		}
	}

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
	if r.contentSize > r.maxContentSize {
		r.headResult, r.failReason = StatusContentTooLarge, "content size exceeds http client's limit"
		return false
	}

	return true
}
func (r *hResponse_) applyHeader(index uint8) bool {
	header := &r.primes[index]
	name := header.nameAt(r.input)
	if sh := &hResponseSingletonHeaderTable[hResponseSingletonHeaderFind(header.hash)]; sh.hash == header.hash && bytes.Equal(sh.name, name) {
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
	} else if mh := &hResponseImportantHeaderTable[hResponseImportantHeaderFind(header.hash)]; mh.hash == header.hash && bytes.Equal(mh.name, name) {
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

var ( // perfect hash table for response singleton headers
	hResponseSingletonHeaderTable = [12]struct {
		parse bool // need general parse or not
		desc       // allowQuote, allowEmpty, allowParam, hasComment
		check func(*hResponse_, *pair, uint8) bool
	}{ // age content-length content-range content-type date etag expires last-modified location retry-after server set-cookie
		0:  {false, desc{hashDate, false, false, false, false, bytesDate}, (*hResponse_).checkDate},
		1:  {false, desc{hashContentLength, false, false, false, false, bytesContentLength}, (*hResponse_).checkContentLength},
		2:  {false, desc{hashAge, false, false, false, false, bytesAge}, (*hResponse_).checkAge},
		3:  {false, desc{hashSetCookie, false, false, false, false, bytesSetCookie}, (*hResponse_).checkSetCookie}, // `a=b; Path=/; HttpsOnly` is not parameters
		4:  {false, desc{hashLastModified, false, false, false, false, bytesLastModified}, (*hResponse_).checkLastModified},
		5:  {false, desc{hashLocation, false, false, false, false, bytesLocation}, (*hResponse_).checkLocation},
		6:  {false, desc{hashExpires, false, false, false, false, bytesExpires}, (*hResponse_).checkExpires},
		7:  {false, desc{hashContentRange, false, false, false, false, bytesContentRange}, (*hResponse_).checkContentRange},
		8:  {false, desc{hashETag, false, false, false, false, bytesETag}, (*hResponse_).checkETag},
		9:  {false, desc{hashServer, false, false, false, true, bytesServer}, (*hResponse_).checkServer},
		10: {true, desc{hashContentType, false, false, true, false, bytesContentType}, (*hResponse_).checkContentType},
		11: {false, desc{hashRetryAfter, false, false, false, false, bytesRetryAfter}, (*hResponse_).checkRetryAfter},
	}
	hResponseSingletonHeaderFind = func(hash uint16) int { return (889344 / int(hash)) % 12 }
)

func (r *hResponse_) checkAge(header *pair, index uint8) bool { // Age = delta-seconds
	if header.value.isEmpty() {
		r.headResult, r.failReason = StatusBadRequest, "empty age"
		return false
	}
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
	// TODO
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
	hResponseImportantHeaderTable = [17]struct {
		desc  // allowQuote, allowEmpty, allowParam, hasComment
		check func(*hResponse_, []pair, uint8, uint8) bool
	}{ // accept-encoding accept-ranges allow alt-svc cache-control cache-status cdn-cache-control connection content-encoding content-language proxy-authenticate trailer transfer-encoding upgrade vary via www-authenticate
		0:  {desc{hashAcceptRanges, false, false, false, false, bytesAcceptRanges}, (*hResponse_).checkAcceptRanges},
		1:  {desc{hashVia, false, false, false, true, bytesVia}, (*hResponse_).checkVia},
		2:  {desc{hashWWWAuthenticate, false, false, false, false, bytesWWWAuthenticate}, (*hResponse_).checkWWWAuthenticate},
		3:  {desc{hashConnection, false, false, false, false, bytesConnection}, (*hResponse_).checkConnection},
		4:  {desc{hashContentEncoding, false, false, false, false, bytesContentEncoding}, (*hResponse_).checkContentEncoding},
		5:  {desc{hashAllow, false, true, false, false, bytesAllow}, (*hResponse_).checkAllow},
		6:  {desc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*hResponse_).checkTransferEncoding}, // deliberately false
		7:  {desc{hashTrailer, false, false, false, false, bytesTrailer}, (*hResponse_).checkTrailer},
		8:  {desc{hashVary, false, false, false, false, bytesVary}, (*hResponse_).checkVary},
		9:  {desc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*hResponse_).checkUpgrade},
		10: {desc{hashProxyAuthenticate, false, false, false, false, bytesProxyAuthenticate}, (*hResponse_).checkProxyAuthenticate},
		11: {desc{hashCacheControl, false, false, false, false, bytesCacheControl}, (*hResponse_).checkCacheControl},
		12: {desc{hashAltSvc, false, false, true, false, bytesAltSvc}, (*hResponse_).checkAltSvc},
		13: {desc{hashCDNCacheControl, false, false, false, false, bytesCDNCacheControl}, (*hResponse_).checkCDNCacheControl},
		14: {desc{hashCacheStatus, false, false, true, false, bytesCacheStatus}, (*hResponse_).checkCacheStatus},
		15: {desc{hashAcceptEncoding, false, true, true, false, bytesAcceptEncoding}, (*hResponse_).checkAcceptEncoding},
		16: {desc{hashContentLanguage, false, false, false, false, bytesContentLanguage}, (*hResponse_).checkContentLanguage},
	}
	hResponseImportantHeaderFind = func(hash uint16) int { return (72189325 / int(hash)) % 17 }
)

func (r *hResponse_) checkAcceptRanges(pairs []pair, from uint8, edge uint8) bool { // Accept-Ranges = 1#range-unit
	if from == edge {
		r.headResult, r.failReason = StatusBadRequest, "accept-ranges = 1#range-unit"
		return false
	}
	for i := from; i < edge; i++ {
		data := pairs[i].dataAt(r.input)
		bytesToLower(data)
		if bytes.Equal(data, bytesBytes) {
			r.acceptBytes = true
		} else {
			// Ignore
		}
	}
	return true
}
func (r *hResponse_) checkAllow(pairs []pair, from uint8, edge uint8) bool { // Allow = #method
	r.hasAllow = true
	if r.zones.allow.isEmpty() {
		r.zones.allow.from = from
	}
	r.zones.allow.edge = edge
	return true
}
func (r *hResponse_) checkAltSvc(pairs []pair, from uint8, edge uint8) bool { // Alt-Svc = clear / 1#alt-value
	if from == edge {
		r.headResult, r.failReason = StatusBadRequest, "alt-svc = clear / 1#alt-value"
		return false
	}
	if r.zones.altSvc.isEmpty() {
		r.zones.altSvc.from = from
	}
	r.zones.altSvc.edge = edge
	return true
}
func (r *hResponse_) checkCacheControl(pairs []pair, from uint8, edge uint8) bool { // Cache-Control = #cache-directive
	// cache-directive = token [ "=" ( token / quoted-string ) ]
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *hResponse_) checkCacheStatus(pairs []pair, from uint8, edge uint8) bool { // ?
	// TODO
	return true
}
func (r *hResponse_) checkCDNCacheControl(pairs []pair, from uint8, edge uint8) bool { // ?
	// TODO
	return true
}
func (r *hResponse_) checkProxyAuthenticate(pairs []pair, from uint8, edge uint8) bool { // Proxy-Authenticate = #challenge
	// TODO
	return true
}
func (r *hResponse_) checkTransferEncoding(pairs []pair, from uint8, edge uint8) bool { // Transfer-Encoding = #transfer-coding
	if r.status < StatusOK || r.status == StatusNoContent {
		r.headResult, r.failReason = StatusBadRequest, "transfer-encoding is not allowed in 1xx and 204 responses"
		return false
	}
	if r.status == StatusNotModified {
		// TODO
	}
	return r.httpIn_.checkTransferEncoding(pairs, from, edge)
}
func (r *hResponse_) checkUpgrade(pairs []pair, from uint8, edge uint8) bool { // Upgrade = #protocol
	// TODO: socket, tcptun, udptun?
	r.headResult, r.failReason = StatusBadRequest, "upgrade is not supported in normal mode"
	return false
}
func (r *hResponse_) checkVary(pairs []pair, from uint8, edge uint8) bool { // Vary = #( "*" / field-name )
	if r.zones.vary.isEmpty() {
		r.zones.vary.from = from
	}
	r.zones.vary.edge = edge
	return true
}
func (r *hResponse_) checkWWWAuthenticate(pairs []pair, from uint8, edge uint8) bool { // WWW-Authenticate = #challenge
	// TODO
	return true
}
func (r *hResponse_) _checkChallenge(pairs []pair, from uint8, edge uint8) bool { // challenge = auth-scheme [ 1*SP ( token68 / [ auth-param *( OWS "," OWS auth-param ) ] ) ]
	// TODO
	return true
}

func (r *hResponse_) parseSetCookie(setCookieString span) bool { // set-cookie-string = cookie-pair *( ";" SP cookie-av )
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

func (r *hResponse_) applyTrailer(index uint8) bool {
	//trailer := &r.primes[index]
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

func (r *hResponse_) addCookie(cookie *cookie) bool {
	// TODO
	return true
}

func (r *hResponse_) saveContentFilesDir() string {
	return r.stream.keeper().SaveContentFilesDir()
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
