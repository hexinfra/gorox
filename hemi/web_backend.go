// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General Web backend implementation. See RFC 9110 and 9111.

package hemi

import (
	"bytes"
	"io"
	"time"
)

// WebBackend
type WebBackend interface { // for *HTTP[1-3]Backend
	// Imports
	Backend
	contentSaver
	// Methods
	SendTimeout() time.Duration // timeout to send the whole message
	RecvTimeout() time.Duration // timeout to recv the whole message content
	MaxContentSize() int64      // in response
	MaxMemoryContentSize() int32
	MaxStreamsPerConn() int32
	FetchStream() (HStream, error)
	StoreStream(stream HStream)
}

// webBackend_ is the parent for HTTP[1-3]Backend.
type webBackend_[N Node] struct {
	// Parent
	Backend_[N]
	// Mixins
	_webKeeper_
	// States
}

func (b *webBackend_[N]) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *webBackend_[N]) OnConfigure() {
	b.Backend_.OnConfigure()
	b._webKeeper_.onConfigure(b, 60*time.Second, 60*time.Second, 1000, TmpsDir()+"/web/backends/"+b.name)

	// sub components
	b.ConfigureNodes()
}
func (b *webBackend_[N]) OnPrepare() {
	b.Backend_.OnPrepare()
	b._webKeeper_.onPrepare(b)

	// sub components
	b.PrepareNodes()
}

// HStream is the backend-side web stream.
type HStream interface { // for *H[1-3]Stream
	Request() HRequest
	Response() HResponse
	Socket() HSocket

	ExecuteExchan() error
	ExecuteSocket() error

	webConn() webConn
	markBroken()
}

// HRequest is the backend-side web request.
type HRequest interface { // for *H[1-3]Request
	setMethodURI(method []byte, uri []byte, hasContent bool) bool
	setAuthority(hostname []byte, colonPort []byte) bool
	proxyCopyCookies(req Request) bool // HTTP 1/2/3 have different requirements on "cookie" header
	proxyCopyHead(req Request, args *WebExchanProxyArgs) bool
	proxyPass(req Request) error
	proxyPost(content any, hasTrailers bool) error
	proxyCopyTail(req Request, args *WebExchanProxyArgs) bool
	isVague() bool
	endVague() error
}

// hRequest_ is the parent for H[1-3]Request.
type hRequest_ struct { // outgoing. needs building
	// Parent
	webOut_ // outgoing web message
	// Assocs
	response HResponse // the corresponding response
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	unixTimes struct { // in seconds
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
		ifUnmodifiedSince uint8
		ifRange           uint8
	}
}

func (r *hRequest_) onUse(versionCode uint8) { // for non-zeros
	const asRequest = true
	r.webOut_.onUse(versionCode, asRequest)
	r.unixTimes.ifModifiedSince = -1   // not set
	r.unixTimes.ifUnmodifiedSince = -1 // not set
}
func (r *hRequest_) onEnd() { // for zeros
	r.hRequest0 = hRequest0{}
	r.webOut_.onEnd()
}

func (r *hRequest_) Response() HResponse { return r.response }

func (r *hRequest_) SetMethodURI(method string, uri string, hasContent bool) bool {
	return r.shell.(HRequest).setMethodURI(ConstBytes(method), ConstBytes(uri), hasContent)
}
func (r *hRequest_) setScheme(scheme []byte) bool { // HTTP/2 and HTTP/3 only. HTTP/1 doesn't use this!
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

func (r *hRequest_) beforeSend() {} // revising is not supported in backend side.
func (r *hRequest_) doSend() error { // revising is not supported in backend side.
	return r.shell.sendChain()
}

func (r *hRequest_) beforeEcho() {} // revising is not supported in backend side.
func (r *hRequest_) doEcho() error { // revising is not supported in backend side.
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	r.chain.PushTail(&r.piece)
	defer r.chain.free()
	return r.shell.echoChain()
}
func (r *hRequest_) endVague() error { // revising is not supported in backend side.
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	return r.shell.finalizeVague()
}

var ( // perfect hash table for request critical headers
	hRequestCriticalHeaderTable = [12]struct {
		hash uint16
		name []byte
		fAdd func(*hRequest_, []byte) (ok bool)
		fDel func(*hRequest_) (deleted bool)
	}{ // connection content-length content-type cookie date host if-modified-since if-range if-unmodified-since transfer-encoding upgrade via
		0:  {hashContentLength, bytesContentLength, nil, nil}, // restricted
		1:  {hashConnection, bytesConnection, nil, nil},       // restricted
		2:  {hashIfRange, bytesIfRange, (*hRequest_).appendIfRange, (*hRequest_).deleteIfRange},
		3:  {hashUpgrade, bytesUpgrade, nil, nil}, // restricted
		4:  {hashIfModifiedSince, bytesIfModifiedSince, (*hRequest_).appendIfModifiedSince, (*hRequest_).deleteIfModifiedSince},
		5:  {hashIfUnmodifiedSince, bytesIfUnmodifiedSince, (*hRequest_).appendIfUnmodifiedSince, (*hRequest_).deleteIfUnmodifiedSince},
		6:  {hashHost, bytesHost, (*hRequest_).appendHost, (*hRequest_).deleteHost},
		7:  {hashTransferEncoding, bytesTransferEncoding, nil, nil}, // restricted
		8:  {hashContentType, bytesContentType, (*hRequest_).appendContentType, (*hRequest_).deleteContentType},
		9:  {hashCookie, bytesCookie, nil, nil}, // restricted
		10: {hashDate, bytesDate, (*hRequest_).appendDate, (*hRequest_).deleteDate},
		11: {hashVia, bytesVia, nil, nil}, // restricted
	}
	hRequestCriticalHeaderFind = func(hash uint16) int { return (645048 / int(hash)) % 12 }
)

func (r *hRequest_) insertHeader(hash uint16, name []byte, value []byte) bool {
	h := &hRequestCriticalHeaderTable[hRequestCriticalHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(h.name, name) {
		if h.fAdd == nil { // mainly because this header is restricted to insert
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
func (r *hRequest_) appendIfUnmodifiedSince(since []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince, bytesIfUnmodifiedSince, since)
}
func (r *hRequest_) appendIfRange(ifRange []byte) (ok bool) {
	return r._appendSingleton(&r.indexes.ifRange, bytesIfRange, ifRange)
}

func (r *hRequest_) removeHeader(hash uint16, name []byte) bool {
	h := &hRequestCriticalHeaderTable[hRequestCriticalHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(h.name, name) {
		if h.fDel == nil { // mainly because this header is restricted to remove
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
func (r *hRequest_) deleteIfUnmodifiedSince() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince)
}
func (r *hRequest_) deleteIfRange() (deleted bool) {
	return r._deleteSingleton(&r.indexes.ifRange)
}

func (r *hRequest_) proxyPass(req Request) error { // sync content to backend directly
	pass := r.shell.passBytes
	if req.IsVague() {
		pass = r.EchoBytes
	} else {
		r.isSent = true
		r.contentSize = req.ContentSize()
		// TODO: find a way to reduce i/o syscalls if content is small?
		if err := r.shell.passHeaders(); err != nil {
			return err
		}
	}
	for {
		p, err := req.readContent()
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
	if req.HasTrailers() { // added trailers will be written by upper code eventually.
		if !req.forTrailers(func(trailer *pair, name []byte, value []byte) bool {
			return r.shell.addTrailer(name, value)
		}) {
			return webOutTrailerFailed
		}
	}
	return nil
}
func (r *hRequest_) proxyCopyHead(req Request, args *WebExchanProxyArgs) bool {
	req.delHopHeaders()

	// copy control (:method, :path, :authority, :scheme)
	uri := req.UnsafeURI()
	if req.IsAsteriskOptions() { // OPTIONS *
		// RFC 9112 (3.2.4):
		// If a proxy receives an OPTIONS request with an absolute-form of request-target in which the URI has an empty path and no query component,
		// then the last proxy on the request chain MUST send a request-target of "*" when it forwards the request to the indicated origin server.
		uri = bytesAsterisk
	}
	if !r.shell.(HRequest).setMethodURI(req.UnsafeMethod(), uri, req.HasContent()) {
		return false
	}
	if req.IsAbsoluteForm() || len(args.Hostname) != 0 || len(args.ColonPort) != 0 { // TODO: what about HTTP/2 and HTTP/3?
		req.unsetHost()
		if req.IsAbsoluteForm() {
			if !r.shell.addHeader(bytesHost, req.UnsafeAuthority()) {
				return false
			}
		} else { // custom authority (hostname or colonPort)
			var (
				hostname  []byte
				colonPort []byte
			)
			if len(args.Hostname) == 0 { // no custom hostname
				hostname = req.UnsafeHostname()
			} else {
				hostname = args.Hostname
			}
			if len(args.ColonPort) == 0 { // no custom colonPort
				colonPort = req.UnsafeColonPort()
			} else {
				colonPort = args.ColonPort
			}
			if !r.shell.(HRequest).setAuthority(hostname, colonPort) {
				return false
			}
		}
	}
	if r.versionCode >= Version2 {
		var scheme []byte
		if r.stream.webConn().IsTLS() {
			scheme = bytesSchemeHTTPS
		} else {
			scheme = bytesSchemeHTTP
		}
		if !r.setScheme(scheme) {
			return false
		}
	} else {
		// we have no way to set scheme in HTTP/1 unless we use absolute-form, which is a risk as many servers may not support it.
	}

	// copy selective forbidden headers (including cookie) from req
	if req.HasCookies() && !r.shell.(HRequest).proxyCopyCookies(req) {
		return false
	}
	if !r.shell.addHeader(bytesVia, args.InboundViaName) { // an HTTP-to-HTTP gateway MUST send an appropriate Via header field in each inbound request message
		return false
	}

	// copy added headers
	for name, vValue := range args.AddRequestHeaders {
		var value []byte
		if vValue.IsVariable() {
			value = vValue.BytesVar(req)
		} else if v, ok := vValue.Bytes(); ok {
			value = v
		} else {
			// Invalid values are treated as empty
		}
		if !r.shell.addHeader(ConstBytes(name), value) {
			return false
		}
	}

	// copy remaining headers from req
	if !req.forHeaders(func(header *pair, name []byte, value []byte) bool {
		return r.shell.insertHeader(header.hash, name, value)
	}) {
		return false
	}

	for _, name := range args.DelRequestHeaders {
		r.shell.delHeader(name)
	}

	return true
}
func (r *hRequest_) proxyCopyTail(req Request, args *WebExchanProxyArgs) bool {
	return req.forTrailers(func(trailer *pair, name []byte, value []byte) bool {
		return r.shell.addTrailer(name, value)
	})
}

// HResponse is the backend-side web response.
type HResponse interface { // for *H[1-3]Response
	KeepAlive() int8
	HeadResult() int16
	BodyResult() int16
	Status() int16
	HasContent() bool
	ContentSize() int64
	HasTrailers() bool
	IsVague() bool
	examineTail() bool
	holdContent() any
	readContent() (p []byte, err error)
	delHopHeaders()
	delHopTrailers()
	forHeaders(callback func(header *pair, name []byte, value []byte) bool) bool
	forTrailers(callback func(header *pair, name []byte, value []byte) bool) bool
	recvHead()
	reuse()
}

// hResponse_ is the parent for H[1-3]Response.
type hResponse_ struct { // incoming. needs parsing
	// Parent
	webIn_ // incoming web message
	// Stream states (stocks)
	stockCookies [8]HCookie // for r.cookies
	// Stream states (controlled)
	cookie HCookie // to overcome the limitation of Go's escape analysis when receiving set-cookie
	// Stream states (non-zeros)
	cookies []HCookie // hold cookies->r.input. [<r.stockCookies>/(make=32/128)]
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
		retryAfter   uint8   // retry-after header ->r.input
		server       uint8   // server header ->r.input
		_            [2]byte // padding
	}
	zones struct { // zones of some selected headers, for fast accessing
		allow  zone
		altSvc zone
		vary   zone
		_      [2]byte // padding
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

func (r *hResponse_) onUse(versionCode uint8) { // for non-zeros
	const asResponse = true
	r.webIn_.onUse(versionCode, asResponse)

	r.cookies = r.stockCookies[0:0:cap(r.stockCookies)] // use append()
}
func (r *hResponse_) onEnd() { // for zeros
	r.cookie.input = nil
	for i := 0; i < len(r.cookies); i++ {
		r.cookies[i].input = nil
	}
	r.cookies = nil
	r.hResponse0 = hResponse0{}

	r.webIn_.onEnd()
}

func (r *hResponse_) reuse() {
	versionCode := r.versionCode
	r.onEnd()
	r.onUse(versionCode)
}

func (r *hResponse_) Status() int16 { return r.status }

func (r *hResponse_) examineHead() bool {
	for i := r.headers.from; i < r.headers.edge; i++ {
		if !r.applyHeader(i) {
			// r.headResult is set.
			return false
		}
	}
	if DebugLevel() >= 2 {
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
	case Version1_0: // we don't support HTTP/1.0 in backend side
		BugExitln("HTTP/1.0 must be denied priorly")
	case Version1_1:
		if r.keepAlive == -1 { // no connection header
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
		} else if !r._parseField(header, &sh.fdesc, r.input, true) { // fully
			r.headResult = StatusBadRequest
			return false
		}
		if !sh.check(r, header, index) {
			// r.headResult is set.
			return false
		}
	} else if mh := &hResponseImportantHeaderTable[hResponseImportantHeaderFind(header.hash)]; mh.hash == header.hash && bytes.Equal(mh.name, name) {
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

var ( // perfect hash table for singleton response headers
	hResponseSingletonHeaderTable = [12]struct {
		parse bool // need general parse or not
		fdesc      // allowQuote, allowEmpty, allowParam, hasComment
		check func(*hResponse_, *pair, uint8) bool
	}{ // age content-length content-range content-type date etag expires last-modified location retry-after server set-cookie
		0:  {false, fdesc{hashDate, false, false, false, false, bytesDate}, (*hResponse_).checkDate},
		1:  {false, fdesc{hashContentLength, false, false, false, false, bytesContentLength}, (*hResponse_).checkContentLength},
		2:  {false, fdesc{hashAge, false, false, false, false, bytesAge}, (*hResponse_).checkAge},
		3:  {false, fdesc{hashSetCookie, false, false, false, false, bytesSetCookie}, (*hResponse_).checkSetCookie}, // `a=b; Path=/; HttpsOnly` is not parameters
		4:  {false, fdesc{hashLastModified, false, false, false, false, bytesLastModified}, (*hResponse_).checkLastModified},
		5:  {false, fdesc{hashLocation, false, false, false, false, bytesLocation}, (*hResponse_).checkLocation},
		6:  {false, fdesc{hashExpires, false, false, false, false, bytesExpires}, (*hResponse_).checkExpires},
		7:  {false, fdesc{hashContentRange, false, false, false, false, bytesContentRange}, (*hResponse_).checkContentRange},
		8:  {false, fdesc{hashETag, false, false, false, false, bytesETag}, (*hResponse_).checkETag},
		9:  {false, fdesc{hashServer, false, false, false, true, bytesServer}, (*hResponse_).checkServer},
		10: {true, fdesc{hashContentType, false, false, true, false, bytesContentType}, (*hResponse_).checkContentType},
		11: {false, fdesc{hashRetryAfter, false, false, false, false, bytesRetryAfter}, (*hResponse_).checkRetryAfter},
	}
	hResponseSingletonHeaderFind = func(hash uint16) int { return (889344 / int(hash)) % 12 }
)

func (r *hResponse_) checkAge(header *pair, index uint8) bool { // Age = delta-seconds
	if header.value.isEmpty() {
		r.headResult, r.failReason = StatusBadRequest, "empty age"
		return false
	}
	// TODO: check
	return true
}
func (r *hResponse_) checkETag(header *pair, index uint8) bool { // ETag = entity-tag
	// TODO: check
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
	// TODO: check
	r.indexes.location = index
	return true
}
func (r *hResponse_) checkRetryAfter(header *pair, index uint8) bool { // Retry-After = HTTP-date / delay-seconds
	// TODO: check
	r.indexes.retryAfter = index
	return true
}
func (r *hResponse_) checkServer(header *pair, index uint8) bool { // Server = product *( RWS ( product / comment ) )
	// TODO: check
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
			cookies := make([]HCookie, 0, 16)
			r.cookies = append(cookies, r.cookies...)
		} else if cap(r.cookies) == 16 {
			cookies := make([]HCookie, 0, 128)
			r.cookies = append(cookies, r.cookies...)
		} else {
			r.headResult = StatusRequestHeaderFieldsTooLarge
			return false
		}
	}
	r.cookies = append(r.cookies, r.cookie)
	return true
}

var ( // perfect hash table for important response headers
	hResponseImportantHeaderTable = [17]struct {
		fdesc // allowQuote, allowEmpty, allowParam, hasComment
		check func(*hResponse_, []pair, uint8, uint8) bool
	}{ // accept-encoding accept-ranges allow alt-svc cache-control cache-status cdn-cache-control connection content-encoding content-language proxy-authenticate trailer transfer-encoding upgrade vary via www-authenticate
		0:  {fdesc{hashAcceptRanges, false, false, false, false, bytesAcceptRanges}, (*hResponse_).checkAcceptRanges},
		1:  {fdesc{hashVia, false, false, false, true, bytesVia}, (*hResponse_).checkVia},
		2:  {fdesc{hashWWWAuthenticate, false, false, false, false, bytesWWWAuthenticate}, (*hResponse_).checkWWWAuthenticate},
		3:  {fdesc{hashConnection, false, false, false, false, bytesConnection}, (*hResponse_).checkConnection},
		4:  {fdesc{hashContentEncoding, false, false, false, false, bytesContentEncoding}, (*hResponse_).checkContentEncoding},
		5:  {fdesc{hashAllow, false, true, false, false, bytesAllow}, (*hResponse_).checkAllow},
		6:  {fdesc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*hResponse_).checkTransferEncoding}, // deliberately false
		7:  {fdesc{hashTrailer, false, false, false, false, bytesTrailer}, (*hResponse_).checkTrailer},
		8:  {fdesc{hashVary, false, false, false, false, bytesVary}, (*hResponse_).checkVary},
		9:  {fdesc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*hResponse_).checkUpgrade},
		10: {fdesc{hashProxyAuthenticate, false, false, false, false, bytesProxyAuthenticate}, (*hResponse_).checkProxyAuthenticate},
		11: {fdesc{hashCacheControl, false, false, false, false, bytesCacheControl}, (*hResponse_).checkCacheControl},
		12: {fdesc{hashAltSvc, false, false, true, false, bytesAltSvc}, (*hResponse_).checkAltSvc},
		13: {fdesc{hashCDNCacheControl, false, false, false, false, bytesCDNCacheControl}, (*hResponse_).checkCDNCacheControl},
		14: {fdesc{hashCacheStatus, false, false, true, false, bytesCacheStatus}, (*hResponse_).checkCacheStatus},
		15: {fdesc{hashAcceptEncoding, false, true, true, false, bytesAcceptEncoding}, (*hResponse_).checkAcceptEncoding},
		16: {fdesc{hashContentLanguage, false, false, false, false, bytesContentLanguage}, (*hResponse_).checkContentLanguage},
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
		bytesToLower(data) // range unit names are case-insensitive
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
	// TODO; use r._checkChallenge
	return true
}
func (r *hResponse_) checkWWWAuthenticate(pairs []pair, from uint8, edge uint8) bool { // WWW-Authenticate = #challenge
	// TODO; use r._checkChallenge
	return true
}
func (r *hResponse_) _checkChallenge(pairs []pair, from uint8, edge uint8) bool { // challenge = auth-scheme [ 1*SP ( token68 / [ auth-param *( OWS "," OWS auth-param ) ] ) ]
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
	return r.webIn_.checkTransferEncoding(pairs, from, edge)
}
func (r *hResponse_) checkUpgrade(pairs []pair, from uint8, edge uint8) bool { // Upgrade = #protocol
	if r.versionCode >= Version2 {
		r.headResult, r.failReason = StatusBadRequest, "upgrade is not supported in http/2 and http/3"
		return false
	}
	// TODO: what about websocket?
	r.headResult, r.failReason = StatusBadRequest, "upgrade is not supported in exchan mode"
	return false
}
func (r *hResponse_) checkVary(pairs []pair, from uint8, edge uint8) bool { // Vary = #( "*" / field-name )
	if r.zones.vary.isEmpty() {
		r.zones.vary.from = from
	}
	r.zones.vary.edge = edge
	return true
}

func (r *hResponse_) parseSetCookie(setCookieString span) bool {
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
	cookie := &r.cookie
	cookie.zero()
	cookie.input = &r.input
	// TODO: parse
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

func (r *hResponse_) HasCookies() bool { return len(r.cookies) > 0 }
func (r *hResponse_) GetCookie(name string) *HCookie {
	for i := 0; i < len(r.cookies); i++ {
		if cookie := &r.cookies[i]; cookie.nameEqualString(name) {
			return cookie
		}
	}
	return nil
}
func (r *hResponse_) HasCookie(name string) bool {
	for i := 0; i < len(r.cookies); i++ {
		if cookie := &r.cookies[i]; cookie.nameEqualString(name) {
			return true
		}
	}
	return false
}
func (r *hResponse_) forCookies(callback func(cookie *HCookie) bool) bool {
	for i := 0; i < len(r.cookies); i++ {
		if !callback(&r.cookies[i]) {
			return false
		}
	}
	return true
}

func (r *hResponse_) HasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified)
	// responses do not include content.
	if r.status < StatusOK || r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might
	// be of zero length.
	return r.contentSize >= 0 || r.IsVague()
}
func (r *hResponse_) Content() string       { return string(r.unsafeContent()) }
func (r *hResponse_) UnsafeContent() []byte { return r.unsafeContent() }

func (r *hResponse_) examineTail() bool {
	for i := r.trailers.from; i < r.trailers.edge; i++ {
		if !r.applyTrailer(i) {
			// r.bodyResult is set.
			return false
		}
	}
	return true
}
func (r *hResponse_) applyTrailer(index uint8) bool {
	//trailer := &r.primes[index]
	// TODO: Pseudo-header fields MUST NOT appear in a trailer section.
	return true
}

// HCookie is a "set-cookie" header received from backend.
type HCookie struct { // 32 bytes
	input      *[]byte // the buffer holding data
	expires    int64   // Expires=Wed, 09 Jun 2021 10:18:14 GMT
	maxAge     int32   // Max-Age=123
	nameFrom   int16   // foo
	valueEdge  int16   // bar
	domainFrom int16   // Domain=example.com
	pathFrom   int16   // Path=/abc
	nameSize   uint8   // <= 255
	domainSize uint8   // <= 255
	pathSize   uint8   // <= 255
	flags      uint8   // secure(1), httpOnly(1), sameSite(2), reserved(2), valueOffset(2)
}

func (c *HCookie) zero() { *c = HCookie{} }

func (c *HCookie) Name() string {
	p := *c.input
	return string(p[c.nameFrom : c.nameFrom+int16(c.nameSize)])
}
func (c *HCookie) Value() string {
	p := *c.input
	valueFrom := c.nameFrom + int16(c.nameSize) + 1 // name=value
	return string(p[valueFrom:c.valueEdge])
}
func (c *HCookie) Expires() int64 { return c.expires }
func (c *HCookie) MaxAge() int32  { return c.maxAge }
func (c *HCookie) domain() []byte {
	p := *c.input
	return p[c.domainFrom : c.domainFrom+int16(c.domainSize)]
}
func (c *HCookie) path() []byte {
	p := *c.input
	return p[c.pathFrom : c.pathFrom+int16(c.pathSize)]
}
func (c *HCookie) sameSite() string {
	switch c.flags & 0b00110000 {
	case 0b00010000:
		return "Lax"
	case 0b00100000:
		return "Strict"
	default:
		return "None"
	}
}
func (c *HCookie) secure() bool   { return c.flags&0b10000000 > 0 }
func (c *HCookie) httpOnly() bool { return c.flags&0b01000000 > 0 }

func (c *HCookie) nameEqualString(name string) bool {
	if int(c.nameSize) != len(name) {
		return false
	}
	p := *c.input
	return string(p[c.nameFrom:c.nameFrom+int16(c.nameSize)]) == name
}
func (c *HCookie) nameEqualBytes(name []byte) bool {
	if int(c.nameSize) != len(name) {
		return false
	}
	p := *c.input
	return bytes.Equal(p[c.nameFrom:c.nameFrom+int16(c.nameSize)], name)
}

// HSocket is the backend-side web socket.
type HSocket interface { // for *H[1-3]Socket
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
}

// hSocket_ is the parent for H[1-3]Socket.
type hSocket_ struct {
	// Parent
	webSocket_
	// Assocs
	// Stream states (zeros)
}

func (s *hSocket_) onUse() {
	s.webSocket_.onUse()
}
func (s *hSocket_) onEnd() {
	s.webSocket_.onEnd()
}
