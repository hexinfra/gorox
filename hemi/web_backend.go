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
	SendTimeout() time.Duration  // timeout to send the whole message
	RecvTimeout() time.Duration  // timeout to recv the whole message content
	MaxContentSize() int64       // in response
	MaxMemoryContentSize() int32 // in response
	MaxStreamsPerConn() int32
	FetchStream() (backendStream, error)
	StoreStream(stream backendStream)
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
	b._webKeeper_.onConfigure(b, 60*time.Second, 60*time.Second, 1000, TmpDir()+"/web/backends/"+b.name)

	// sub components
	b.ConfigureNodes()
}
func (b *webBackend_[N]) OnPrepare() {
	b.Backend_.OnPrepare()
	b._webKeeper_.onPrepare(b)

	// sub components
	b.PrepareNodes()
}

// backendStream is the backend-side web stream.
type backendStream interface { // for *backend[1-3]Stream
	Request() backendRequest
	Response() backendResponse
	Socket() backendSocket

	ExecuteExchan() error
	ExecuteSocket() error

	webConn() webConn
	isBroken() bool
	markBroken()
}

// backendRequest is the backend-side web request.
type backendRequest interface { // for *backend[1-3]Request
	setMethodURI(method []byte, uri []byte, hasContent bool) bool
	setAuthority(hostname []byte, colonPort []byte) bool
	proxyCopyCookies(req Request) bool // HTTP 1/2/3 have different requirements on "cookie" header
	proxyCopyHead(req Request, cfg *WebExchanProxyConfig) bool
	proxyPass(req Request) error
	proxyPost(content any, hasTrailers bool) error
	proxyCopyTail(req Request, cfg *WebExchanProxyConfig) bool
	isVague() bool
	endVague() error
}

// backendRequest_ is the parent for backend[1-3]Request.
type backendRequest_ struct { // outgoing. needs building
	// Parent
	webOut_ // outgoing web message
	// Assocs
	response backendResponse // the corresponding response
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	unixTimes struct { // in seconds
		ifModifiedSince   int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
		ifUnmodifiedSince int64 // -1: not set, -2: set through general api, >= 0: set unix time in seconds
	}
	// Stream states (zeros)
	backendRequest0 // all values must be zero by default in this struct!
}
type backendRequest0 struct { // for fast reset, entirely
	indexes struct {
		host              uint8
		ifModifiedSince   uint8
		ifUnmodifiedSince uint8
		ifRange           uint8
	}
}

func (r *backendRequest_) onUse(httpVersion uint8) { // for non-zeros
	const asRequest = true
	r.webOut_.onUse(httpVersion, asRequest)
	r.unixTimes.ifModifiedSince = -1   // not set
	r.unixTimes.ifUnmodifiedSince = -1 // not set
}
func (r *backendRequest_) onEnd() { // for zeros
	r.backendRequest0 = backendRequest0{}
	r.webOut_.onEnd()
}

func (r *backendRequest_) Response() backendResponse { return r.response }

func (r *backendRequest_) SetMethodURI(method string, uri string, hasContent bool) bool {
	return r.shell.(backendRequest).setMethodURI(ConstBytes(method), ConstBytes(uri), hasContent)
}
func (r *backendRequest_) setScheme(scheme []byte) bool { // HTTP/2 and HTTP/3 only. HTTP/1 doesn't use this!
	// TODO: copy `:scheme $scheme` to r.fields
	return false
}
func (r *backendRequest_) control() []byte { return r.fields[0:r.controlEdge] } // TODO: maybe we need a struct type to represent pseudo headers?

func (r *backendRequest_) SetIfModifiedSince(since int64) bool {
	return r._setUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince, since)
}
func (r *backendRequest_) SetIfUnmodifiedSince(since int64) bool {
	return r._setUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince, since)
}

func (r *backendRequest_) beforeSend() {} // revising is not supported in backend side.
func (r *backendRequest_) doSend() error { // revising is not supported in backend side.
	return r.shell.sendChain()
}

func (r *backendRequest_) beforeEcho() {} // revising is not supported in backend side.
func (r *backendRequest_) doEcho() error { // revising is not supported in backend side.
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	r.chain.PushTail(&r.piece)
	defer r.chain.free()
	return r.shell.echoChain()
}
func (r *backendRequest_) endVague() error { // revising is not supported in backend side.
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	return r.shell.finalizeVague()
}

var ( // perfect hash table for request critical headers
	backendRequestCriticalHeaderTable = [12]struct {
		hash uint16
		name []byte
		fAdd func(*backendRequest_, []byte) (ok bool)
		fDel func(*backendRequest_) (deleted bool)
	}{ // connection content-length content-type cookie date host if-modified-since if-range if-unmodified-since transfer-encoding upgrade via
		0:  {hashContentLength, bytesContentLength, nil, nil}, // restricted
		1:  {hashConnection, bytesConnection, nil, nil},       // restricted
		2:  {hashIfRange, bytesIfRange, (*backendRequest_).appendIfRange, (*backendRequest_).deleteIfRange},
		3:  {hashUpgrade, bytesUpgrade, nil, nil}, // restricted
		4:  {hashIfModifiedSince, bytesIfModifiedSince, (*backendRequest_).appendIfModifiedSince, (*backendRequest_).deleteIfModifiedSince},
		5:  {hashIfUnmodifiedSince, bytesIfUnmodifiedSince, (*backendRequest_).appendIfUnmodifiedSince, (*backendRequest_).deleteIfUnmodifiedSince},
		6:  {hashHost, bytesHost, (*backendRequest_).appendHost, (*backendRequest_).deleteHost},
		7:  {hashTransferEncoding, bytesTransferEncoding, nil, nil}, // restricted
		8:  {hashContentType, bytesContentType, (*backendRequest_).appendContentType, (*backendRequest_).deleteContentType},
		9:  {hashCookie, bytesCookie, nil, nil}, // restricted
		10: {hashDate, bytesDate, (*backendRequest_).appendDate, (*backendRequest_).deleteDate},
		11: {hashVia, bytesVia, nil, nil}, // restricted
	}
	backendRequestCriticalHeaderFind = func(hash uint16) int { return (645048 / int(hash)) % 12 }
)

func (r *backendRequest_) insertHeader(hash uint16, name []byte, value []byte) bool {
	h := &backendRequestCriticalHeaderTable[backendRequestCriticalHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(h.name, name) {
		if h.fAdd == nil { // mainly because this header is restricted to insert
			return true // pretend to be successful
		}
		return h.fAdd(r, value)
	}
	return r.shell.addHeader(name, value)
}
func (r *backendRequest_) appendHost(host []byte) (ok bool) {
	return r._appendSingleton(&r.indexes.host, bytesHost, host)
}
func (r *backendRequest_) appendIfModifiedSince(since []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince, bytesIfModifiedSince, since)
}
func (r *backendRequest_) appendIfUnmodifiedSince(since []byte) (ok bool) {
	return r._addUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince, bytesIfUnmodifiedSince, since)
}
func (r *backendRequest_) appendIfRange(ifRange []byte) (ok bool) {
	return r._appendSingleton(&r.indexes.ifRange, bytesIfRange, ifRange)
}

func (r *backendRequest_) removeHeader(hash uint16, name []byte) bool {
	h := &backendRequestCriticalHeaderTable[backendRequestCriticalHeaderFind(hash)]
	if h.hash == hash && bytes.Equal(h.name, name) {
		if h.fDel == nil { // mainly because this header is restricted to remove
			return true // pretend to be successful
		}
		return h.fDel(r)
	}
	return r.shell.delHeader(name)
}
func (r *backendRequest_) deleteHost() (deleted bool) {
	return r._deleteSingleton(&r.indexes.host)
}
func (r *backendRequest_) deleteIfModifiedSince() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.ifModifiedSince, &r.indexes.ifModifiedSince)
}
func (r *backendRequest_) deleteIfUnmodifiedSince() (deleted bool) {
	return r._delUnixTime(&r.unixTimes.ifUnmodifiedSince, &r.indexes.ifUnmodifiedSince)
}
func (r *backendRequest_) deleteIfRange() (deleted bool) {
	return r._deleteSingleton(&r.indexes.ifRange)
}

func (r *backendRequest_) proxyPass(req Request) error { // sync content to backend directly
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
func (r *backendRequest_) proxyCopyHead(req Request, cfg *WebExchanProxyConfig) bool {
	req.delHopHeaders()

	// copy control (:method, :path, :authority, :scheme)
	uri := req.UnsafeURI()
	if req.IsAsteriskOptions() { // OPTIONS *
		// RFC 9112 (3.2.4):
		// If a proxy receives an OPTIONS request with an absolute-form of request-target in which the URI has an empty path and no query component,
		// then the last proxy on the request chain MUST send a request-target of "*" when it forwards the request to the indicated origin server.
		uri = bytesAsterisk
	}
	if !r.shell.(backendRequest).setMethodURI(req.UnsafeMethod(), uri, req.HasContent()) {
		return false
	}
	if req.IsAbsoluteForm() || len(cfg.Hostname) != 0 || len(cfg.ColonPort) != 0 { // TODO: what about HTTP/2 and HTTP/3?
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
			if len(cfg.Hostname) == 0 { // no custom hostname
				hostname = req.UnsafeHostname()
			} else {
				hostname = cfg.Hostname
			}
			if len(cfg.ColonPort) == 0 { // no custom colonPort
				colonPort = req.UnsafeColonPort()
			} else {
				colonPort = cfg.ColonPort
			}
			if !r.shell.(backendRequest).setAuthority(hostname, colonPort) {
				return false
			}
		}
	}
	if r.httpVersion >= Version2 {
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
	if req.HasCookies() && !r.shell.(backendRequest).proxyCopyCookies(req) {
		return false
	}
	if !r.shell.addHeader(bytesVia, cfg.InboundViaName) { // an HTTP-to-HTTP gateway MUST send an appropriate Via header field in each inbound request message
		return false
	}

	// copy added headers
	for name, vValue := range cfg.AddRequestHeaders {
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

	for _, name := range cfg.DelRequestHeaders {
		r.shell.delHeader(name)
	}

	return true
}
func (r *backendRequest_) proxyCopyTail(req Request, cfg *WebExchanProxyConfig) bool {
	return req.forTrailers(func(trailer *pair, name []byte, value []byte) bool {
		return r.shell.addTrailer(name, value)
	})
}

// backendResponse is the backend-side web response.
type backendResponse interface { // for *backend[1-3]Response
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

// backendResponse_ is the parent for backend[1-3]Response.
type backendResponse_ struct { // incoming. needs parsing
	// Parent
	webIn_ // incoming web message
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	backendResponse0 // all values must be zero by default in this struct!
}
type backendResponse0 struct { // for fast reset, entirely
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

func (r *backendResponse_) onUse(httpVersion uint8) { // for non-zeros
	const asResponse = true
	r.webIn_.onUse(httpVersion, asResponse)
}
func (r *backendResponse_) onEnd() { // for zeros
	r.backendResponse0 = backendResponse0{}

	r.webIn_.onEnd()
}

func (r *backendResponse_) reuse() {
	httpVersion := r.httpVersion
	r.onEnd()
	r.onUse(httpVersion)
}

func (r *backendResponse_) Status() int16 { return r.status }

func (r *backendResponse_) examineHead() bool {
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
	switch r.httpVersion {
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
func (r *backendResponse_) applyHeader(index uint8) bool {
	header := &r.primes[index]
	name := header.nameAt(r.input)
	if sh := &backendResponseSingletonHeaderTable[backendResponseSingletonHeaderFind(header.hash)]; sh.hash == header.hash && bytes.Equal(sh.name, name) {
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
	} else if mh := &backendResponseImportantHeaderTable[backendResponseImportantHeaderFind(header.hash)]; mh.hash == header.hash && bytes.Equal(mh.name, name) {
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
	backendResponseSingletonHeaderTable = [12]struct {
		parse bool // need general parse or not
		fdesc      // allowQuote, allowEmpty, allowParam, hasComment
		check func(*backendResponse_, *pair, uint8) bool
	}{ // age content-length content-range content-type date etag expires last-modified location retry-after server set-cookie
		0:  {false, fdesc{hashDate, false, false, false, false, bytesDate}, (*backendResponse_).checkDate},
		1:  {false, fdesc{hashContentLength, false, false, false, false, bytesContentLength}, (*backendResponse_).checkContentLength},
		2:  {false, fdesc{hashAge, false, false, false, false, bytesAge}, (*backendResponse_).checkAge},
		3:  {false, fdesc{hashSetCookie, false, false, false, false, bytesSetCookie}, (*backendResponse_).checkSetCookie}, // `a=b; Path=/; HttpsOnly` is not parameters
		4:  {false, fdesc{hashLastModified, false, false, false, false, bytesLastModified}, (*backendResponse_).checkLastModified},
		5:  {false, fdesc{hashLocation, false, false, false, false, bytesLocation}, (*backendResponse_).checkLocation},
		6:  {false, fdesc{hashExpires, false, false, false, false, bytesExpires}, (*backendResponse_).checkExpires},
		7:  {false, fdesc{hashContentRange, false, false, false, false, bytesContentRange}, (*backendResponse_).checkContentRange},
		8:  {false, fdesc{hashETag, false, false, false, false, bytesETag}, (*backendResponse_).checkETag},
		9:  {false, fdesc{hashServer, false, false, false, true, bytesServer}, (*backendResponse_).checkServer},
		10: {true, fdesc{hashContentType, false, false, true, false, bytesContentType}, (*backendResponse_).checkContentType},
		11: {false, fdesc{hashRetryAfter, false, false, false, false, bytesRetryAfter}, (*backendResponse_).checkRetryAfter},
	}
	backendResponseSingletonHeaderFind = func(hash uint16) int { return (889344 / int(hash)) % 12 }
)

func (r *backendResponse_) checkAge(header *pair, index uint8) bool { // Age = delta-seconds
	if header.value.isEmpty() {
		r.headResult, r.failReason = StatusBadRequest, "empty age"
		return false
	}
	// TODO: check
	return true
}
func (r *backendResponse_) checkETag(header *pair, index uint8) bool { // ETag = entity-tag
	// TODO: check
	r.indexes.etag = index
	return true
}
func (r *backendResponse_) checkExpires(header *pair, index uint8) bool { // Expires = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.expires, &r.unixTimes.expires)
}
func (r *backendResponse_) checkLastModified(header *pair, index uint8) bool { // Last-Modified = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.lastModified, &r.unixTimes.lastModified)
}
func (r *backendResponse_) checkLocation(header *pair, index uint8) bool { // Location = URI-reference
	// TODO: check
	r.indexes.location = index
	return true
}
func (r *backendResponse_) checkRetryAfter(header *pair, index uint8) bool { // Retry-After = HTTP-date / delay-seconds
	// TODO: check
	r.indexes.retryAfter = index
	return true
}
func (r *backendResponse_) checkServer(header *pair, index uint8) bool { // Server = product *( RWS ( product / comment ) )
	// TODO: check
	r.indexes.server = index
	return true
}
func (r *backendResponse_) checkSetCookie(header *pair, index uint8) bool { // Set-Cookie = set-cookie-string
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

var ( // perfect hash table for important response headers
	backendResponseImportantHeaderTable = [17]struct {
		fdesc // allowQuote, allowEmpty, allowParam, hasComment
		check func(*backendResponse_, []pair, uint8, uint8) bool
	}{ // accept-encoding accept-ranges allow alt-svc cache-control cache-status cdn-cache-control connection content-encoding content-language proxy-authenticate trailer transfer-encoding upgrade vary via www-authenticate
		0:  {fdesc{hashAcceptRanges, false, false, false, false, bytesAcceptRanges}, (*backendResponse_).checkAcceptRanges},
		1:  {fdesc{hashVia, false, false, false, true, bytesVia}, (*backendResponse_).checkVia},
		2:  {fdesc{hashWWWAuthenticate, false, false, false, false, bytesWWWAuthenticate}, (*backendResponse_).checkWWWAuthenticate},
		3:  {fdesc{hashConnection, false, false, false, false, bytesConnection}, (*backendResponse_).checkConnection},
		4:  {fdesc{hashContentEncoding, false, false, false, false, bytesContentEncoding}, (*backendResponse_).checkContentEncoding},
		5:  {fdesc{hashAllow, false, true, false, false, bytesAllow}, (*backendResponse_).checkAllow},
		6:  {fdesc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*backendResponse_).checkTransferEncoding}, // deliberately false
		7:  {fdesc{hashTrailer, false, false, false, false, bytesTrailer}, (*backendResponse_).checkTrailer},
		8:  {fdesc{hashVary, false, false, false, false, bytesVary}, (*backendResponse_).checkVary},
		9:  {fdesc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*backendResponse_).checkUpgrade},
		10: {fdesc{hashProxyAuthenticate, false, false, false, false, bytesProxyAuthenticate}, (*backendResponse_).checkProxyAuthenticate},
		11: {fdesc{hashCacheControl, false, false, false, false, bytesCacheControl}, (*backendResponse_).checkCacheControl},
		12: {fdesc{hashAltSvc, false, false, true, false, bytesAltSvc}, (*backendResponse_).checkAltSvc},
		13: {fdesc{hashCDNCacheControl, false, false, false, false, bytesCDNCacheControl}, (*backendResponse_).checkCDNCacheControl},
		14: {fdesc{hashCacheStatus, false, false, true, false, bytesCacheStatus}, (*backendResponse_).checkCacheStatus},
		15: {fdesc{hashAcceptEncoding, false, true, true, false, bytesAcceptEncoding}, (*backendResponse_).checkAcceptEncoding},
		16: {fdesc{hashContentLanguage, false, false, false, false, bytesContentLanguage}, (*backendResponse_).checkContentLanguage},
	}
	backendResponseImportantHeaderFind = func(hash uint16) int { return (72189325 / int(hash)) % 17 }
)

func (r *backendResponse_) checkAcceptRanges(pairs []pair, from uint8, edge uint8) bool { // Accept-Ranges = 1#range-unit
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
func (r *backendResponse_) checkAllow(pairs []pair, from uint8, edge uint8) bool { // Allow = #method
	r.hasAllow = true
	if r.zones.allow.isEmpty() {
		r.zones.allow.from = from
	}
	r.zones.allow.edge = edge
	return true
}
func (r *backendResponse_) checkAltSvc(pairs []pair, from uint8, edge uint8) bool { // Alt-Svc = clear / 1#alt-value
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
func (r *backendResponse_) checkCacheControl(pairs []pair, from uint8, edge uint8) bool { // Cache-Control = #cache-directive
	// cache-directive = token [ "=" ( token / quoted-string ) ]
	for i := from; i < edge; i++ {
		// TODO
	}
	return true
}
func (r *backendResponse_) checkCacheStatus(pairs []pair, from uint8, edge uint8) bool { // ?
	// TODO
	return true
}
func (r *backendResponse_) checkCDNCacheControl(pairs []pair, from uint8, edge uint8) bool { // ?
	// TODO
	return true
}
func (r *backendResponse_) checkProxyAuthenticate(pairs []pair, from uint8, edge uint8) bool { // Proxy-Authenticate = #challenge
	// TODO; use r._checkChallenge
	return true
}
func (r *backendResponse_) checkWWWAuthenticate(pairs []pair, from uint8, edge uint8) bool { // WWW-Authenticate = #challenge
	// TODO; use r._checkChallenge
	return true
}
func (r *backendResponse_) _checkChallenge(pairs []pair, from uint8, edge uint8) bool { // challenge = auth-scheme [ 1*SP ( token68 / [ auth-param *( OWS "," OWS auth-param ) ] ) ]
	// TODO
	return true
}
func (r *backendResponse_) checkTransferEncoding(pairs []pair, from uint8, edge uint8) bool { // Transfer-Encoding = #transfer-coding
	if r.status < StatusOK || r.status == StatusNoContent {
		r.headResult, r.failReason = StatusBadRequest, "transfer-encoding is not allowed in 1xx and 204 responses"
		return false
	}
	if r.status == StatusNotModified {
		// TODO
	}
	return r.webIn_.checkTransferEncoding(pairs, from, edge)
}
func (r *backendResponse_) checkUpgrade(pairs []pair, from uint8, edge uint8) bool { // Upgrade = #protocol
	if r.httpVersion >= Version2 {
		r.headResult, r.failReason = StatusBadRequest, "upgrade is not supported in http/2 and http/3"
		return false
	}
	// TODO: what about websocket?
	r.headResult, r.failReason = StatusBadRequest, "upgrade is not supported in exchan mode"
	return false
}
func (r *backendResponse_) checkVary(pairs []pair, from uint8, edge uint8) bool { // Vary = #( "*" / field-name )
	if r.zones.vary.isEmpty() {
		r.zones.vary.from = from
	}
	r.zones.vary.edge = edge
	return true
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

func (r *backendResponse_) HasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified)
	// responses do not include content.
	if r.status < StatusOK || r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might
	// be of zero length.
	return r.contentSize >= 0 || r.IsVague()
}
func (r *backendResponse_) Content() string       { return string(r.unsafeContent()) }
func (r *backendResponse_) UnsafeContent() []byte { return r.unsafeContent() }

func (r *backendResponse_) examineTail() bool {
	for i := r.trailers.from; i < r.trailers.edge; i++ {
		if !r.applyTrailer(i) {
			// r.bodyResult is set.
			return false
		}
	}
	return true
}
func (r *backendResponse_) applyTrailer(index uint8) bool {
	//trailer := &r.primes[index]
	// TODO: Pseudo-header fields MUST NOT appear in a trailer section.
	return true
}

// backendSocket is the backend-side web socket.
type backendSocket interface { // for *backend[1-3]Socket
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
}

// backendSocket_ is the parent for backend[1-3]Socket.
type backendSocket_ struct {
	// Parent
	webSocket_
	// Assocs
	// Stream states (zeros)
}

func (s *backendSocket_) onUse() {
	s.webSocket_.onUse()
}
func (s *backendSocket_) onEnd() {
	s.webSocket_.onEnd()
}
