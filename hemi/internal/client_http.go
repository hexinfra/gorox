// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General HTTP client implementation.

package internal

import (
	"bytes"
	"sync/atomic"
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

func (c *httpClient_) MaxContentSize() int64 { return c.maxContentSize }

// httpOutgate_ is the mixin for HTTP[1-3]Outgate.
type httpOutgate_ struct {
	// Mixins
	outgate_
	httpClient_
	// States
}

func (f *httpOutgate_) init(name string, stage *Stage) {
	f.outgate_.init(name, stage)
}

func (f *httpOutgate_) configure() {
	f.outgate_.configure()
	// maxStreamsPerConn
	f.ConfigureInt32("maxStreamsPerConn", &f.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
	// saveContentFilesDir
	f.ConfigureString("saveContentFilesDir", &f.saveContentFilesDir, func(value string) bool { return value != "" }, TempDir()+"/outgates/"+f.name)
	// maxContentSize
	f.ConfigureInt64("maxContentSize", &f.maxContentSize, func(value int64) bool { return value > 0 }, _1T)
}
func (f *httpOutgate_) prepare() {
	f.outgate_.prepare()
	f.makeContentFilesDir(0755)
}
func (f *httpOutgate_) shutdown() {
	f.outgate_.shutdown()
}

// httpBackend_ is the mixin for HTTP[1-3]Backend.
type httpBackend_ struct {
	// Mixins
	backend_
	httpClient_
	// States
	healthCheck any // TODO
}

func (b *httpBackend_) init(name string, stage *Stage) {
	b.backend_.init(name, stage)
}

func (b *httpBackend_) configure() {
	b.backend_.configure()
	// maxStreamsPerConn
	b.ConfigureInt32("maxStreamsPerConn", &b.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
	// saveContentFilesDir
	b.ConfigureString("saveContentFilesDir", &b.saveContentFilesDir, func(value string) bool { return value != "" }, TempDir()+"/backends/"+b.name)
	// maxContentSize
	b.ConfigureInt64("maxContentSize", &b.maxContentSize, func(value int64) bool { return value > 0 }, _1T)
}
func (b *httpBackend_) prepare(numNodes int) {
	b.backend_.prepare(numNodes)
	b.makeContentFilesDir(0755)
}
func (b *httpBackend_) shutdown() {
	b.backend_.shutdown()
}

// httpNode_ is the mixin for http[1-3]Node.
type httpNode_ struct {
	// Mixins
	node_
	// States
	//something related to http?
}

func (n *httpNode_) init(id int32) {
	n.node_.init(id)
}

// hConn is the interface for *H[1-3]Conn.
type hConn interface {
	getClient() httpClient
	isBroken() bool
	markBroken()
	makeTempName(p []byte, seconds int64) (from int, edge int) // small enough to be placed in smallStack() of stream
}

// hConn_ is the mixin for H[1-3]Conn.
type hConn_ struct {
	// Mixins
	conn_
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
	counter     int64 // used to make temp name
	usedStreams int32 // how many streams has been used?
	broken      int32 // use sync/atomic
}

func (c *hConn_) onGet(id int64, client httpClient) {
	c.conn_.onGet(id, client)
}
func (c *hConn_) onPut() {
	c.conn_.onPut()
	c.counter = 0
	c.usedStreams = 0
	atomic.StoreInt32(&c.broken, 0)
}

func (c *hConn_) getClient() httpClient { return c.client.(httpClient) }

func (c *hConn_) reachLimit() bool {
	return atomic.AddInt32(&c.usedStreams, 1) > c.getClient().MaxStreamsPerConn()
}

func (c *hConn_) isBroken() bool { return atomic.LoadInt32(&c.broken) == 1 }
func (c *hConn_) markBroken()    { atomic.StoreInt32(&c.broken, 1) }

func (c *hConn_) makeTempName(p []byte, seconds int64) (from int, edge int) {
	return makeTempName(p, int64(c.client.Stage().ID()), c.id, seconds, atomic.AddInt64(&c.counter, 1))
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
	setControl(method []byte, uri []byte) bool
	addHeader(name []byte, value []byte) bool
}

// hRequest_ is the mixin for H[1-3]Request.
type hRequest_ struct {
	// Mixins
	httpOutMessage_
	// Assocs
	response response // the corresponding response
	// Stream states (buffers)
	stockCookies [4]nava
	// Stream states (controlled)
	// Stream states (non-zeros)
	cookies []nava // hold cookies->r.array? [<r.stockCookies>/?]
	// Stream states (zeros)
	hRequest0_ // all values must be zero by default in this struct!
}
type hRequest0_ struct { // for fast reset, entirely
}

func (r *hRequest_) onUse() { // for non-zeros
	r.httpOutMessage_.onUse()
}
func (r *hRequest_) onEnd() { // for zeros
	r.hRequest0_ = hRequest0_{}
	r.httpOutMessage_.onEnd()
}

func (r *hRequest_) addCookie(name []byte, value []byte) bool {
	// TODO: to r.array?
	return false
}

func (r *hRequest_) send() error {
	return r.shell.doSend(r.content)
}

func (r *hRequest_) checkPush() error {
	if r.stream.isBroken() {
		return httpWriteBroken
	}
	if r.isSent {
		return nil
	}
	if r.contentSize != -1 {
		return httpMixIdentityChunked
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
	return r.shell.doPush(curChain)
}
func (r *hRequest_) finishPush() error {
	if r.stream.isBroken() {
		return httpWriteBroken
	}
	return r.shell.pushEnd()
}

func (r *hRequest_) withHead(req Request) bool { // used by proxies
	var uri []byte
	if req.isServerOptions() { // OPTIONS *
		// RFC 9112 (3.2.4):
		// If a proxy receives an OPTIONS request with an absolute-form of request-target in which the URI has an empty path and no query component,
		// then the last proxy on the request chain MUST send a request-target of "*" when it forwards the request to the indicated origin server.

		// Note, even target form is asterisk-form, not absolute-form, req.uri is still empty in our implementation, not "*". So just use "*".
		uri = httpBytesAsterisk
	} else {
		uri = req.UnsafeURI()
	}
	if !r.shell.(request).setControl(req.UnsafeMethod(), uri) {
		return false
	}
	// copy critical headers from req
	req.delCriticalHeaders()
	req.delHopHeaders()
	if req.isAbsoluteForm() {
		// When a proxy receives a request with an absolute-form of request-target, the proxy MUST ignore the received Host header
		// field (if any) and instead replace it with the host information of the request-target. A proxy that forwards such a request
		// MUST generate a new Host field value based on the received request-target rather than forward the received Host field value.
		req.delHost()
		if !r.shell.addHeader(httpBytesHost, req.UnsafeAuthority()) {
			return false
		}
	}
	if !req.walkHeaders(func(name []byte, value []byte) bool {
		return r.shell.addHeader(name, value)
	}, false) {
		return false
	}
	// TODO: add date?
	return true
}
func (r *hRequest_) pass(req Request) error { // used by proxies.
	// TODO
	// content may be identity (write through) or chunked (use push)
	return nil
}
func (r *hRequest_) passTrailers(req Request) bool { // used by proxies
	// TODO
	return false
}

func (r *hRequest_) Response() response { return r.response }

func (r *hRequest_) isForbiddenField(hash uint16, name []byte) bool {
	return httpIsForbiddenRequestField(hash, name)
}

var ( // forbidden request fields
	httpForbiddenRequestFields = [3]struct { // TODO: perfect hashing
		hash uint16
		name []byte
	}{
		0: {httpHashConnection, httpBytesConnection},
		1: {httpHashContentLength, httpBytesContentLength},
		2: {httpHashTransferEncoding, httpBytesTransferEncoding},
	}
	httpIsForbiddenRequestField = func(hash uint16, name []byte) bool {
		// TODO: perfect hashing
		for _, field := range httpForbiddenRequestFields {
			if field.hash == hash && bytes.Equal(field.name, name) {
				return true
			}
		}
		return false
	}
)

// response is the client-side HTTP response and interface for *H[1-3]Response.
type response interface {
	Status() int16
	ContentSize() int64
	unsafeDate() []byte
	unsafeETag() []byte
	unsafeLastModified() []byte
	hasTrailers() bool
	delCriticalHeaders()
	delHopHeaders()
	delHopTrailers()
	walkHeaders(fn func(name []byte, value []byte) bool, withConnection bool) bool
	walkTrailers(fn func(name []byte, value []byte) bool, withConnection bool) bool
	recvContent(retain bool) any
}

// hResponse_ is the mixin for H[1-3]Response.
type hResponse_ struct {
	// Mixins
	httpInMessage_
	// Stream states (buffers)
	stockCookies [4]cookie // for r.cookies
	// Stream states (controlled)
	// Stream states (non-zeros)
	cookies []cookie // hold cookies->r.input. [<r.stockCookies>/?]
	// Stream states (zeros)
	hResponse0_ // all values must be zero by default in this struct!
}
type hResponse0_ struct { // for fast reset, entirely
	status           int16 // 200, 302, 404, ...
	dateTime         int64 // parsed unix timestamp of date
	lastModifiedTime int64 // parsed unix timestamp of last-modified
	cacheControl     struct {
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
	indexes struct {
		server       uint8 // server header ->r.input
		date         uint8 // date header ->r.input
		lastModified uint8 // last-modified header ->r.input
		etag         uint8 // etag header ->r.input
		acceptRanges uint8 // accept-ranges header ->r.input
	}
}

func (r *hResponse_) onUse() { // for non-zeros
	r.httpInMessage_.onUse(true)
	r.cookies = r.stockCookies[0:0:cap(r.stockCookies)] // use append()
}
func (r *hResponse_) onEnd() { // for zeros
	if cap(r.cookies) != cap(r.stockCookies) {
		// put
		r.cookies = nil
	}
	r.hResponse0_ = hResponse0_{}
	r.httpInMessage_.onEnd()
}

func (r *hResponse_) arrayCopy(p []byte) bool {
	if len(p) > 0 {
		edge := r.arrayEdge + int32(len(p))
		if edge < r.arrayEdge { // overflow
			return false
		}
		/*
			if r.app != nil && edge > r.app.maxMemoryContentSize {
				return false
			}
		*/
		if !r._growArray(int32(len(p))) {
			return false
		}
		r.arrayEdge += int32(copy(r.array[r.arrayEdge:], p))
	}
	return true
}

func (r *hResponse_) Status() int16 { return r.status }

func (r *hResponse_) useHeader(header *pair) bool {
	headerName := r.input[header.nameFrom : header.nameFrom+int32(header.nameSize)]
	if h := &httpMultipleResponseHeaderTable[httpMultipleResponseHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(httpMultipleResponseHeaderBytes[h.from:h.edge], headerName) {
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
		if h := &httpCriticalResponseHeaderTable[httpCriticalResponseHeaderFind(header.hash)]; h.hash == header.hash && bytes.Equal(httpCriticalResponseHeaderBytes[h.from:h.edge], headerName) {
			if h.check != nil && !h.check(r, header, r.headers.edge-1) {
				// r.headResult is set.
				return false
			}
		}
	}
	return true
}

var ( // perfect hash table for multiple response headers
	httpMultipleResponseHeaderBytes = []byte("accept-encoding accept-ranges allow cache-control connection content-encoding trailer transfer-encoding upgrade vary via www-authenticate")
	httpMultipleResponseHeaderTable = [12]struct {
		hash  uint16
		from  uint8
		edge  uint8
		must  bool // true if 1#, false if #
		check func(*hResponse_, uint8, uint8) bool
	}{
		0:  {httpHashVary, 112, 116, false, nil},
		1:  {httpHashUpgrade, 104, 111, false, (*hResponse_).checkUpgrade},
		2:  {httpHashWWWAuthenticate, 121, 137, false, nil},
		3:  {httpHashAllow, 30, 35, false, nil},
		4:  {httpHashConnection, 50, 60, false, (*hResponse_).checkConnection},
		5:  {httpHashAcceptEncoding, 0, 15, false, nil},
		6:  {httpHashVia, 117, 120, false, nil},
		7:  {httpHashCacheControl, 36, 49, false, (*hResponse_).checkCacheControl},
		8:  {httpHashTransferEncoding, 86, 103, false, (*hResponse_).checkTransferEncoding},
		9:  {httpHashAcceptRanges, 16, 29, false, nil},
		10: {httpHashTrailer, 78, 85, false, nil},
		11: {httpHashContentEncoding, 61, 77, false, (*hResponse_).checkContentEncoding},
	}
	httpMultipleResponseHeaderFind = func(hash uint16) int { return (750969 / int(hash)) % 12 }
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

var ( // perfect hash table for critical response headers
	httpCriticalResponseHeaderBytes = []byte("content-length content-range content-type date etag expires last-modified location server set-cookie")
	httpCriticalResponseHeaderTable = [10]struct {
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
	httpCriticalResponseHeaderFind = func(hash uint16) int { return (68805 / int(hash)) % 10 }
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
func (h *hResponse_) checkExpires(header *pair, index uint8) bool {
	// TODO
	return true
}
func (r *hResponse_) checkLastModified(header *pair, index uint8) bool {
	// Last-Modified = HTTP-date
	return r._checkHTTPDate(header, index, &r.indexes.lastModified, &r.lastModifiedTime)
}
func (r *hResponse_) checkLocation(header *pair, index uint8) bool {
	// TODO
	return true
}
func (r *hResponse_) checkServer(header *pair, index uint8) bool {
	r.indexes.server = index
	return true
}
func (r *hResponse_) checkSetCookie(header *pair, index uint8) bool {
	// TODO
	return true
}

func (r *hResponse_) checkHead() bool {
	// Resolve r.keepAlive
	if r.keepAlive == -1 { // no connection header
		if r.versionCode == Version1_1 {
			r.keepAlive = 1 // default is keep-alive for HTTP/1.1
		} else {
			r.keepAlive = 0 // default is close for HTTP/1.0
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

func (r *hResponse_) parseSetCookie(setCookieString text) bool {
	// TODO
	return false
}
func (r *hResponse_) addSetCookie(setCookie *cookie) bool {
	// TODO
	r.headResult = StatusRequestHeaderFieldsTooLarge
	return false
}

func (r *hResponse_) unsafeDate() []byte { // used by proxies
	if r.indexes.date == 0 {
		return nil
	}
	vDate := r.primes[r.indexes.date].value
	return r.input[vDate.from:vDate.edge]
}
func (r *hResponse_) unsafeLastModified() []byte { // used by proxies
	if r.indexes.lastModified == 0 {
		return nil
	}
	vDate := r.primes[r.indexes.lastModified].value
	return r.input[vDate.from:vDate.edge]
}
func (r *hResponse_) unsafeETag() []byte { // used by proxies
	if r.indexes.etag == 0 {
		return nil
	}
	vETag := r.primes[r.indexes.etag].value
	return r.input[vETag.from:vETag.edge]
}
func (r *hResponse_) delCriticalHeaders() { // used by proxies
	r.delPrimeAt(r.indexes.server)
	r.delPrimeAt(r.indexes.date)
	r.delPrimeAt(r.indexes.lastModified)
	r.delPrimeAt(r.indexes.etag)
}
func (r *hResponse_) delHopHeaders() { // used by proxies
	r._delHopFields(r.headers, r.delHeader)
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
func (r *hResponse_) Content() string {
	return string(r.UnsafeContent())
}
func (r *hResponse_) UnsafeContent() []byte {
	r.loadContent()
	if r.stream.isBroken() {
		return nil
	}
	return r.contentBlob[0:r.sizeReceived]
}

func (r *hResponse_) useTrailer(trailer *pair) bool {
	// TODO: check trailer?
	return true
}
func (r *hResponse_) delHopTrailers() { // used by proxies
	r._delHopFields(r.trailers, r.delTrailer)
}

func (r *hResponse_) getSaveContentFilesDir() string {
	return r.stream.getHolder().(httpClient).SaveContentFilesDir() // must ends with '/'
}

// cookie is a cookie received from server.
type cookie struct { // 24 bytes. refers to r.input
	nameFrom     int16
	valueFrom    int16
	valueEdge    int16
	expireFrom   int16
	maxAgeFrom   int16
	domainFrom   int16
	pathFrom     int16
	sameSiteFrom int16
	secure       bool
	httpOnly     bool
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
