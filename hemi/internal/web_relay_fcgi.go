// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// FCGI relay implementation.

// FCGI is mainly used by PHP applications. It doesn't support HTTP trailers.
// And we don't use request-side chunking due to the limitation of CGI/1.1 even
// though FCGI can do that through its framing protocol. Perhaps most FCGI
// applications don't implement this feature either.

// In response side, FCGI applications mostly use "vague" output.

// To avoid ambiguity, the term "content" in FCGI specification is called "payload" in our implementation.

package internal

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/common/risky"
)

func init() {
	RegisterHandlet("fcgiRelay", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(fcgiRelay)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// fcgiRelay handlet passes web requests to backend FCGI servers and cache responses.
type fcgiRelay struct {
	// Mixins
	Handlet_
	contentSaver_ // so responses can save their large contents in local file system.
	// Assocs
	stage   *Stage       // current stage
	webapp  *Webapp      // the webapp to which the relay belongs
	backend *TCPSBackend // the backend to pass to
	cacher  Cacher       // the cacher which is used by this relay
	// States
	bufferClientContent bool              // client content is buffered anyway?
	bufferServerContent bool              // server content is buffered anyway?
	keepConn            bool              // instructs FCGI server to keep conn?
	preferUnderscore    bool              // if header name "foo-bar" and "foo_bar" are both present, prefer "foo_bar" to "foo-bar"?
	scriptFilename      []byte            // for SCRIPT_FILENAME
	indexFile           []byte            // for indexFile
	addRequestHeaders   map[string]Value  // headers appended to client request
	delRequestHeaders   [][]byte          // client request headers to delete
	addResponseHeaders  map[string]string // headers appended to server response
	delResponseHeaders  [][]byte          // server response headers to delete
	sendTimeout         time.Duration     // timeout to send the whole request
	recvTimeout         time.Duration     // timeout to recv the whole response content
	maxContentSize      int64             // max response content size allowed
}

func (h *fcgiRelay) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *fcgiRelay) OnShutdown() {
	h.webapp.SubDone()
}

func (h *fcgiRelay) OnConfigure() {
	h.contentSaver_.onConfigure(h, TmpsDir()+"/web/fcgi/"+h.name)

	// toBackend
	if v, ok := h.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := h.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				h.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for fcgiRelay, must be TCPSBackend\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for fcgiRelay")
	}

	// withCacher
	if v, ok := h.Find("withCacher"); ok {
		if name, ok := v.String(); ok && name != "" {
			if cacher := h.stage.Cacher(name); cacher == nil {
				UseExitf("unknown cacher: '%s'\n", name)
			} else {
				h.cacher = cacher
			}
		} else {
			UseExitln("invalid withCacher")
		}
	}

	// bufferClientContent
	h.ConfigureBool("bufferClientContent", &h.bufferClientContent, true)
	// bufferServerContent
	h.ConfigureBool("bufferServerContent", &h.bufferServerContent, true)
	// keepConn
	h.ConfigureBool("keepConn", &h.keepConn, false)
	// preferUnderscore
	h.ConfigureBool("preferUnderscore", &h.preferUnderscore, false)
	// scriptFilename
	h.ConfigureBytes("scriptFilename", &h.scriptFilename, nil, nil)

	// indexFile
	h.ConfigureBytes("indexFile", &h.indexFile, func(value []byte) error {
		if len(value) > 0 {
			return nil
		}
		return errors.New(".indexFile has an invalid value")
	}, []byte("index.php"))

	// sendTimeout
	h.ConfigureDuration("sendTimeout", &h.sendTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, 60*time.Second)

	// recvTimeout
	h.ConfigureDuration("recvTimeout", &h.recvTimeout, func(value time.Duration) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, 60*time.Second)

	// maxContentSize
	h.ConfigureInt64("maxContentSize", &h.maxContentSize, func(value int64) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxContentSize has an invalid value")
	}, _1T)
}
func (h *fcgiRelay) OnPrepare() {
	h.contentSaver_.onPrepare(h, 0755)
}

func (h *fcgiRelay) IsProxy() bool { return true }
func (h *fcgiRelay) IsCache() bool { return h.cacher != nil }

func (h *fcgiRelay) Handle(req Request, resp Response) (handled bool) { // reverse only
	var (
		content  any
		fConn    *TConn
		fErr     error
		fContent any
	)

	hasContent := req.HasContent()
	if hasContent && (h.bufferClientContent || req.IsVague()) { // including size 0
		content = req.takeContent()
		if content == nil { // take failed
			// exchan is marked as broken
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return true
		}
	}

	if h.keepConn {
		if fConn, fErr = h.backend.FetchConn(); fErr == nil {
			defer h.backend.StoreConn(fConn)
		}
	} else {
		if fConn, fErr = h.backend.Dial(); fErr == nil {
			defer fConn.Close()
		}
	}
	if fErr != nil {
		resp.SendBadGateway(nil)
		return true
	}

	fExchan := getFCGIExchan(h, fConn)
	defer putFCGIExchan(fExchan)

	fReq := &fExchan.request
	if !fReq.copyHeadFrom(req, h.scriptFilename, h.indexFile) {
		fExchan.markBroken()
		resp.SendBadGateway(nil)
		return true
	}
	if hasContent && !h.bufferClientContent && !req.IsVague() {
		fErr = fReq.pass(req)
	} else { // nil, []byte, tempFile
		fErr = fReq.post(content)
	}
	if fErr != nil {
		fExchan.markBroken()
		resp.SendBadGateway(nil)
		return true
	}

	fResp := &fExchan.response
	for { // until we found a non-1xx status (>= 200)
		fResp.recvHead()
		if fResp.headResult != StatusOK || fResp.status == StatusSwitchingProtocols { // websocket is not served in handlets.
			fExchan.markBroken()
			resp.SendBadGateway(nil)
			return true
		}
		if fResp.status >= StatusOK {
			break
		}
		// We got 1xx
		if req.VersionCode() == Version1_0 {
			fExchan.markBroken()
			resp.SendBadGateway(nil)
			return true
		}
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !resp.pass1xx(fResp) {
			fExchan.markBroken()
			return true
		}
		fResp.onEnd()
		fResp.onUse()
	}

	fHasContent := false // TODO: if fcgi server includes a content even for HEAD method, what should we do?
	if req.MethodCode() != MethodHEAD {
		fHasContent = fResp.hasContent()
	}
	if fHasContent && h.bufferServerContent { // including size 0
		fContent = fResp.takeContent()
		if fContent == nil { // take failed
			// fExchan is marked as broken
			resp.SendBadGateway(nil)
			return true
		}
	}

	if !resp.copyHeadFrom(fResp, nil) { // viaName = nil
		fExchan.markBroken()
		return true
	}
	if fHasContent && !h.bufferServerContent {
		if err := resp.pass(fResp); err != nil {
			fExchan.markBroken()
			return true
		}
	} else if err := resp.post(fContent, false); err != nil { // false means no trailers
		return true
	}

	return true
}

// poolFCGIExchan
var poolFCGIExchan sync.Pool

func getFCGIExchan(relay *fcgiRelay, conn *TConn) *fcgiExchan {
	var exchan *fcgiExchan
	if x := poolFCGIExchan.Get(); x == nil {
		exchan = new(fcgiExchan)
		req, resp := &exchan.request, &exchan.response
		req.exchan = exchan
		req.response = resp
		resp.exchan = exchan
	} else {
		exchan = x.(*fcgiExchan)
	}
	exchan.onUse(relay, conn)
	return exchan
}
func putFCGIExchan(exchan *fcgiExchan) {
	exchan.onEnd()
	poolFCGIExchan.Put(exchan)
}

// fcgiExchan
type fcgiExchan struct {
	// Assocs
	request  fcgiRequest  // the fcgi request
	response fcgiResponse // the fcgi response
	// Exchan states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	relay  *fcgiRelay // associated relay
	conn   *TConn     // associated conn
	region Region     // a region-based memory pool
	// Exchan states (zeros)
}

func (x *fcgiExchan) onUse(relay *fcgiRelay, conn *TConn) {
	x.relay = relay
	x.conn = conn
	x.region.Init()
	x.request.onUse()
	x.response.onUse()
}
func (x *fcgiExchan) onEnd() {
	x.request.onEnd()
	x.response.onEnd()
	x.region.Free()
	x.conn = nil
	x.relay = nil
}

func (x *fcgiExchan) buffer256() []byte          { return x.stockBuffer[:] }
func (x *fcgiExchan) unsafeMake(size int) []byte { return x.region.Make(size) }

func (x *fcgiExchan) makeTempName(p []byte, unixTime int64) int {
	return x.conn.MakeTempName(p, unixTime)
}

func (x *fcgiExchan) setWriteDeadline(deadline time.Time) error {
	return x.conn.SetWriteDeadline(deadline)
}
func (x *fcgiExchan) setReadDeadline(deadline time.Time) error {
	return x.conn.SetReadDeadline(deadline)
}

func (x *fcgiExchan) write(p []byte) (int, error)                { return x.conn.Write(p) }
func (x *fcgiExchan) writev(vector *net.Buffers) (int64, error)  { return x.conn.Writev(vector) }
func (x *fcgiExchan) read(p []byte) (int, error)                 { return x.conn.Read(p) }
func (x *fcgiExchan) readAtLeast(p []byte, min int) (int, error) { return x.conn.ReadAtLeast(p, min) }

func (x *fcgiExchan) isBroken() bool { return x.conn.IsBroken() }
func (x *fcgiExchan) markBroken()    { x.conn.MarkBroken() }

// fcgiRequest
type fcgiRequest struct { // outgoing. needs building
	// Assocs
	exchan   *fcgiExchan
	response *fcgiResponse
	// Exchan states (stocks)
	stockParams [_2K]byte // for r.params
	// Exchan states (controlled)
	paramsHeader [8]byte // used by params record
	stdinHeader  [8]byte // used by stdin record
	// Exchan states (non-zeros)
	params      []byte        // place the payload of exactly one FCGI_PARAMS record. [<r.stockParams>/16K]
	sendTimeout time.Duration // timeout to send the whole request
	// Exchan states (zeros)
	sendTime     time.Time   // the time when first send operation is performed
	vector       net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after exchan
	fixedVector  [7][]byte   // for sending request. reset after exchan. 120B
	fcgiRequest0             // all values must be zero by default in this struct!
}
type fcgiRequest0 struct { // for fast reset, entirely
	paramsEdge    uint16 // edge of r.params. max size of r.params must be <= 16K.
	forbidContent bool   // forbid content?
	forbidFraming bool   // forbid content-length and transfer-encoding?
}

func (r *fcgiRequest) onUse() {
	copy(r.paramsHeader[:], fcgiEmptyParams) // payloadLen (r.paramsHeader[4:6]) needs modification on using
	copy(r.stdinHeader[:], fcgiEmptyStdin)   // payloadLen (r.stdinHeader[4:6]) needs modification for every stdin record on using
	r.params = r.stockParams[:]
	r.sendTimeout = r.exchan.relay.sendTimeout
}
func (r *fcgiRequest) onEnd() {
	if cap(r.params) != cap(r.stockParams) {
		PutNK(r.params)
		r.params = nil
	}
	r.sendTime = time.Time{}
	r.vector = nil
	r.fixedVector = [7][]byte{}

	r.fcgiRequest0 = fcgiRequest0{}
}

func (r *fcgiRequest) copyHeadFrom(req Request, scriptFilename, indexFile []byte) bool {
	// Add meta params
	if !r._addMetaParam(fcgiBytesGatewayInterface, fcgiBytesCGI1_1) { // GATEWAY_INTERFACE
		return false
	}
	if !r._addMetaParam(fcgiBytesServerSoftware, bytesGorox) { // SERVER_SOFTWARE
		return false
	}
	if !r._addMetaParam(fcgiBytesServerProtocol, req.UnsafeVersion()) { // SERVER_PROTOCOL
		return false
	}
	if !r._addMetaParam(fcgiBytesRequestMethod, req.UnsafeMethod()) { // REQUEST_METHOD
		return false
	}
	if !r._addMetaParam(fcgiBytesRequestScheme, req.UnsafeScheme()) { // REQUEST_SCHEME
		return false
	}
	if !r._addMetaParam(fcgiBytesRequestURI, req.UnsafeURI()) { // REQUEST_URI
		return false
	}
	if !r._addMetaParam(fcgiBytesServerName, req.UnsafeHostname()) { // SERVER_NAME
		return false
	}
	if !r._addMetaParam(fcgiBytesRedirectStatus, fcgiBytes200) { // REDIRECT_STATUS
		return false
	}
	if req.IsHTTPS() && !r._addMetaParam(fcgiBytesHTTPS, fcgiBytesON) { // HTTPS
		return false
	}
	if len(scriptFilename) == 0 {
		absPath := req.unsafeAbsPath()
		if absPath[len(absPath)-1] == '/' && len(indexFile) > 0 {
			scriptFilename = req.UnsafeMake(len(absPath) + len(indexFile))
			copy(scriptFilename, absPath)
			copy(scriptFilename[len(absPath):], indexFile)
		} else {
			scriptFilename = absPath
		}
	}
	if !r._addMetaParam(fcgiBytesScriptFilename, scriptFilename) { // SCRIPT_FILENAME
		return false
	}
	if !r._addMetaParam(fcgiBytesScriptName, req.UnsafePath()) { // SCRIPT_NAME
		return false
	}
	var value []byte
	if value = req.UnsafeQueryString(); len(value) > 1 {
		value = value[1:] // excluding '?'
	} else {
		value = nil
	}
	if !r._addMetaParam(fcgiBytesQueryString, value) { // QUERY_STRING
		return false
	}
	if value = req.UnsafeContentLength(); value != nil && !r._addMetaParam(fcgiBytesContentLength, value) { // CONTENT_LENGTH
		return false
	}
	if value = req.UnsafeContentType(); value != nil && !r._addMetaParam(fcgiBytesContentType, value) { // CONTENT_TYPE
		return false
	}

	// Add http params
	if !req.forHeaders(func(header *pair, name []byte, value []byte) bool {
		return r._addHTTPParam(header, name, value)
	}) {
		return false
	}

	r.paramsHeader[4], r.paramsHeader[5] = byte(r.paramsEdge>>8), byte(r.paramsEdge)

	return true
}
func (r *fcgiRequest) _addMetaParam(name []byte, value []byte) bool { // like: REQUEST_METHOD
	return r._addParam(name, value, false)
}
func (r *fcgiRequest) _addHTTPParam(header *pair, name []byte, value []byte) bool { // like: HTTP_USER_AGENT
	if !header.isUnderscore() || !r.exchan.relay.preferUnderscore {
		return r._addParam(name, value, true)
	}
	// TODO: got a "foo_bar" and user prefer it. avoid name conflicts with header which is like "foo-bar"
	return true
}
func (r *fcgiRequest) _addParam(name []byte, value []byte, http bool) bool { // into r.params
	nameLen, valueLen := len(name), len(value)
	if http {
		nameLen += len(fcgiBytesHTTP_)
	}
	paramSize := 1 + 1 + nameLen + valueLen
	if nameLen > 127 {
		paramSize += 3
	}
	if valueLen > 127 {
		paramSize += 3
	}
	from, edge, ok := r._growParams(paramSize)
	if !ok {
		return false
	}

	if nameLen > 127 {
		r.params[from] = byte(nameLen>>24) | 0x80
		r.params[from+1] = byte(nameLen >> 16)
		r.params[from+2] = byte(nameLen >> 8)
		from += 3
	}
	r.params[from] = byte(nameLen)
	from++
	if valueLen > 127 {
		r.params[from] = byte(valueLen>>24) | 0x80
		r.params[from+1] = byte(valueLen >> 16)
		r.params[from+2] = byte(valueLen >> 8)
		from += 3
	}
	r.params[from] = byte(valueLen)
	from++

	if http { // TODO: improve performance
		from += copy(r.params[from:], fcgiBytesHTTP_)
		last := from + copy(r.params[from:], name)
		for i := from; i < last; i++ {
			if b := r.params[i]; b >= 'a' && b <= 'z' {
				r.params[i] = b - 0x20 // to upper
			} else if b == '-' {
				r.params[i] = '_'
			}
		}
		from = last
	} else {
		from += copy(r.params[from:], name)
	}
	from += copy(r.params[from:], value)
	if from != edge {
		BugExitln("fcgi: from != edge")
	}
	return true
}
func (r *fcgiRequest) _growParams(size int) (from int, edge int, ok bool) { // to place more params into r.params[from:edge]
	if size <= 0 || size > _16K { // size allowed: (0, 16K]
		BugExitln("invalid size in growParams")
	}
	from = int(r.paramsEdge)
	last := r.paramsEdge + uint16(size)
	if last > _16K || last < r.paramsEdge {
		// Overflow
		return
	}
	if last > uint16(cap(r.params)) { // last <= _16K
		params := Get16K()
		copy(params, r.params[:r.paramsEdge])
		r.params = params
	}
	r.paramsEdge = last
	edge, ok = int(r.paramsEdge), true
	return
}

func (r *fcgiRequest) pass(req Request) error { // only for sized (>0) content. vague content must use post(), as we don't use request-side chunking
	r.vector = r.fixedVector[0:4]
	r._setBeginRequest(&r.vector[0])
	r.vector[1] = r.paramsHeader[:]
	r.vector[2] = r.params[:r.paramsEdge] // effective params
	r.vector[3] = fcgiEmptyParams
	if err := r._writeVector(); err != nil {
		return err
	}
	for {
		stdin, err := req.readContent()
		if len(stdin) > 0 {
			size := len(stdin)
			r.stdinHeader[4], r.stdinHeader[5] = byte(size>>8), byte(size)
			if err == io.EOF { // EOF is immediate, write with emptyStdin
				r.vector = r.fixedVector[0:3]
				r.vector[0] = r.stdinHeader[:]
				r.vector[1] = stdin
				r.vector[2] = fcgiEmptyStdin
				return r._writeVector()
			}
			// EOF is not immediate, err must be nil.
			r.vector = r.fixedVector[0:2]
			r.vector[0] = r.stdinHeader[:]
			r.vector[1] = stdin
			if e := r._writeVector(); e != nil {
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
	return r._writeBytes(fcgiEmptyStdin)
}
func (r *fcgiRequest) post(content any) error { // nil, []byte, and *os.File. for bufferClientContent or vague Request content
	if contentText, ok := content.([]byte); ok { // text, <= 64K1
		return r.sendText(contentText)
	} else if contentFile, ok := content.(*os.File); ok { // file
		fileInfo, err := contentFile.Stat()
		if err != nil {
			contentFile.Close()
			return err
		}
		return r.sendFile(contentFile, fileInfo)
	} else { // nil means no content.
		return r.sendText(nil)
	}
}

func (r *fcgiRequest) sendText(content []byte) error { // content <= 64K1
	size := len(content)
	if size == 0 { // beginRequest + (params + emptyParams) + emptyStdin
		r.vector = r.fixedVector[0:5]
	} else { // beginRequest + (params + emptyParams) + (stdin + emptyStdin)
		r.vector = r.fixedVector[0:7]
	}
	// beginRequest
	r._setBeginRequest(&r.vector[0])
	// params + emptyParams
	r.vector[1] = r.paramsHeader[:]
	r.vector[2] = r.params[:r.paramsEdge]
	r.vector[3] = fcgiEmptyParams
	if size == 0 { // emptyStdin
		r.vector[4] = fcgiEmptyStdin
	} else { // stdin + emptyStdin
		r.stdinHeader[4], r.stdinHeader[5] = byte(size>>8), byte(size)
		r.vector[4] = r.stdinHeader[:]
		r.vector[5] = content
		r.vector[6] = fcgiEmptyStdin
	}
	return r._writeVector()
}
func (r *fcgiRequest) sendFile(content *os.File, info os.FileInfo) error {
	buffer := Get16K() // 16K is a tradeoff between performance and memory consumption.
	defer PutNK(buffer)

	sizeRead, fileSize := int64(0), info.Size()
	headSent, lastPart := false, false

	for { // we don't use sendfile(2).
		if sizeRead == fileSize {
			return nil
		}
		readSize := int64(cap(buffer))
		if sizeLeft := fileSize - sizeRead; sizeLeft <= readSize {
			readSize = sizeLeft
			lastPart = true
		}
		n, err := content.ReadAt(buffer[:readSize], sizeRead)
		sizeRead += int64(n)
		if err != nil && sizeRead != fileSize {
			r.exchan.markBroken()
			return err
		}

		if headSent {
			if lastPart { // stdin + emptyStdin
				r.vector = r.fixedVector[0:3]
				r.vector[2] = fcgiEmptyStdin
			} else { // stdin
				r.vector = r.fixedVector[0:2]
			}
			r.stdinHeader[4], r.stdinHeader[5] = byte(n>>8), byte(n)
			r.vector[0] = r.stdinHeader[:]
			r.vector[1] = buffer[:n]
		} else { // head is not sent
			headSent = true
			if lastPart { // beginRequest + (params + emptyParams) + (stdin * N + emptyStdin)
				r.vector = r.fixedVector[0:7]
				r.vector[6] = fcgiEmptyStdin
			} else { // beginRequest + (params + emptyParams) + stdin
				r.vector = r.fixedVector[0:6]
			}
			r._setBeginRequest(&r.vector[0])
			r.vector[1] = r.paramsHeader[:]
			r.vector[2] = r.params[:r.paramsEdge]
			r.vector[3] = fcgiEmptyParams
			r.stdinHeader[4], r.stdinHeader[5] = byte(n>>8), byte(n)
			r.vector[4] = r.stdinHeader[:]
			r.vector[5] = buffer[:n]
		}
		if err = r._writeVector(); err != nil {
			return err
		}
	}
}

func (r *fcgiRequest) _setBeginRequest(p *[]byte) {
	if r.exchan.relay.keepConn {
		*p = fcgiBeginKeepConn
	} else {
		*p = fcgiBeginDontKeep
	}
}

func (r *fcgiRequest) _writeBytes(p []byte) error {
	if r.exchan.isBroken() {
		return fcgiWriteBroken
	}
	if len(p) == 0 {
		return nil
	}
	if err := r._beforeWrite(); err != nil {
		r.exchan.markBroken()
		return err
	}
	_, err := r.exchan.write(p)
	return r._slowCheck(err)
}
func (r *fcgiRequest) _writeVector() error {
	if r.exchan.isBroken() {
		return fcgiWriteBroken
	}
	if len(r.vector) == 1 && len(r.vector[0]) == 0 {
		return nil
	}
	if err := r._beforeWrite(); err != nil {
		r.exchan.markBroken()
		return err
	}
	_, err := r.exchan.writev(&r.vector)
	return r._slowCheck(err)
}
func (r *fcgiRequest) _beforeWrite() error {
	now := time.Now()
	if r.sendTime.IsZero() {
		r.sendTime = now
	}
	return r.exchan.setWriteDeadline(now.Add(r.exchan.relay.backend.WriteTimeout()))
}
func (r *fcgiRequest) _slowCheck(err error) error {
	if err == nil && r._tooSlow() {
		err = fcgiWriteTooSlow
	}
	if err != nil {
		r.exchan.markBroken()
	}
	return err
}
func (r *fcgiRequest) _tooSlow() bool {
	return r.sendTimeout > 0 && time.Now().Sub(r.sendTime) >= r.sendTimeout
}

var ( // fcgi request errors
	fcgiWriteTooSlow = errors.New("fcgi: write too slow")
	fcgiWriteBroken  = errors.New("fcgi: write broken")
)

// fcgiResponse must implements webIn and response interface.
type fcgiResponse struct { // incoming. needs parsing
	// Assocs
	exchan *fcgiExchan
	// Exchan states (stocks)
	stockRecords [8456]byte // for r.records. fcgiHeaderSize + 8K + fcgiMaxPadding. good for PHP
	stockInput   [_2K]byte  // for r.input
	stockPrimes  [48]pair   // for r.primes
	stockExtras  [16]pair   // for r.extras
	// Exchan states (controlled)
	header pair // to overcome the limitation of Go's escape analysis when receiving headers
	// Exchan states (non-zeros)
	records        []byte        // bytes of incoming fcgi records. [<r.stockRecords>/fcgiMaxRecords]
	input          []byte        // bytes of incoming response headers. [<r.stockInput>/4K/16K]
	primes         []pair        // prime fcgi response headers
	extras         []pair        // extra fcgi response headers
	recvTimeout    time.Duration // timeout to recv the whole response content
	maxContentSize int64         // max content size allowed for current response
	status         int16         // 200, 302, 404, ...
	headResult     int16         // result of receiving response head. values are same as http status for convenience
	bodyResult     int16         // result of receiving response body. values are same as http status for convenience
	// Exchan states (zeros)
	failReason    string    // the reason of headResult or bodyResult
	recvTime      time.Time // the time when receiving response
	bodyTime      time.Time // the time when first body read operation is performed on this exchan
	contentText   []byte    // if loadable, the received and loaded content of current response is at r.contentText[:r.receivedSize]
	contentFile   *os.File  // used by r.takeContent(), if content is tempFile. will be closed on exchan ends
	fcgiResponse0           // all values must be zero by default in this struct!
}
type fcgiResponse0 struct { // for fast reset, entirely
	recordsFrom     int32    // from position of current records
	recordsEdge     int32    // edge position of current records
	stdoutFrom      int32    // if stdout's payload is too large to be appended to r.input, use this to note current from position
	stdoutEdge      int32    // see above, to note current edge position
	pBack           int32    // element begins from. for parsing header elements
	pFore           int32    // element spanning to. for parsing header elements
	head            span     // for debugging
	imme            span     // immediate bytes in r.input that belongs to content, not headers
	hasExtra        [8]bool  // see kindXXX for indexes
	inputEdge       int32    // edge position of r.input
	receiving       int8     // currently receiving. see webSectionXXX
	contentTextKind int8     // kind of current r.contentText. see webContentTextXXX
	receivedSize    int64    // bytes of currently received content
	indexes         struct { // indexes of some selected singleton headers, for fast accessing
		contentType  uint8
		date         uint8
		etag         uint8
		expires      uint8
		lastModified uint8
		location     uint8
		xPoweredBy   uint8
		_            byte // padding
	}
	zones struct { // zones of some selected headers, for fast accessing
		allow           zone
		contentLanguage zone
		_               [4]byte // padding
	}
	unixTimes struct { // parsed unix times in seconds
		date         int64 // parsed unix time of date
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

func (r *fcgiResponse) onUse() {
	r.records = r.stockRecords[:]
	r.input = r.stockInput[:]
	r.primes = r.stockPrimes[0:1:cap(r.stockPrimes)] // use append(). r.primes[0] is skipped due to zero value of header indexes.
	r.extras = r.stockExtras[0:0:cap(r.stockExtras)] // use append()
	r.recvTimeout = r.exchan.relay.recvTimeout
	r.maxContentSize = r.exchan.relay.maxContentSize
	r.status = StatusOK
	r.headResult = StatusOK
	r.bodyResult = StatusOK
}
func (r *fcgiResponse) onEnd() {
	if cap(r.records) != cap(r.stockRecords) {
		putFCGIRecords(r.records)
		r.records = nil
	}
	if cap(r.input) != cap(r.stockInput) {
		PutNK(r.input)
		r.input = nil
	}
	if cap(r.primes) != cap(r.stockPrimes) {
		putPairs(r.primes)
		r.primes = nil
	}
	if cap(r.extras) != cap(r.stockExtras) {
		putPairs(r.extras)
		r.extras = nil
	}

	r.failReason = ""
	r.recvTime = time.Time{}
	r.bodyTime = time.Time{}

	if r.contentTextKind == webContentTextPool {
		PutNK(r.contentText)
	}
	r.contentText = nil // other content text kinds are only references, just reset.

	if r.contentFile != nil {
		r.contentFile.Close()
		if Debug() >= 2 {
			Println("contentFile is left as is, not removed!")
		} else if err := os.Remove(r.contentFile.Name()); err != nil {
			// TODO: log?
		}
		r.contentFile = nil
	}

	r.fcgiResponse0 = fcgiResponse0{}
}

func (r *fcgiResponse) recvHead() {
	// The entire response head must be received within one timeout
	if err := r._beforeRead(&r.recvTime); err != nil {
		r.headResult = -1
		return
	}
	if !r.growHead() { // r.input must be empty because we don't use pipelining in requests.
		// r.headResult is set.
		return
	}
	if !r.recvHeaders() || !r.examineHead() { // there is no control in FCGI, or, control is included in headers
		// r.headResult is set.
		return
	}
	r.cleanInput()
}
func (r *fcgiResponse) growHead() bool { // we need more head data to be appended to r.input from r.records
	// Is r.input already full?
	if inputSize := int32(cap(r.input)); r.inputEdge == inputSize { // r.inputEdge reached end, so yes
		if inputSize == _16K { // max r.input size is 16K, we cannot use a larger input anymore
			r.headResult = StatusRequestHeaderFieldsTooLarge
			return false
		}
		// r.input size < 16K. We switch to a larger input (stock -> 4K -> 16K)
		stockSize := int32(cap(r.stockInput))
		var input []byte
		if inputSize == stockSize {
			input = Get4K()
		} else { // 4K
			input = Get16K()
		}
		copy(input, r.input) // copy all
		if inputSize != stockSize {
			PutNK(r.input)
		}
		r.input = input // a larger input is now used
	}
	// r.input is not full. Are there any existing stdout data in r.records?
	if r.stdoutFrom == r.stdoutEdge { // no, we must receive a new non-empty stdout record
		if from, edge, err := r.fcgiRecvStdout(); err == nil {
			r.stdoutFrom, r.stdoutEdge = from, edge
		} else { // unexpected error or EOF
			r.headResult = -1
			return false
		}
	}
	// There are some existing stdout data in r.records now, copy them to r.input
	freeSize := int32(cap(r.input)) - r.inputEdge
	haveSize := r.stdoutEdge - r.stdoutFrom
	copy(r.input[r.inputEdge:], r.records[r.stdoutFrom:r.stdoutEdge])
	if freeSize < haveSize { // stdout data is too much to be placed in r.input
		r.inputEdge += freeSize
		r.stdoutFrom += freeSize
	} else { // freeSize >= haveSize, we have taken all stdout data into r.input
		r.inputEdge += haveSize
		r.stdoutFrom, r.stdoutEdge = 0, 0
	}
	return true
}
func (r *fcgiResponse) recvHeaders() bool { // 1*( field-name ":" OWS field-value OWS CRLF ) CRLF
	// generic-response = 1*header-field NL [ response-body ]
	// header-field    = CGI-field | generic-field
	// CGI-field       = Content-Type | Location | Status
	// Content-Type    = "Content-Type:" media-type NL
	// Status          = "Status:" status-code SP reason-phrase NL
	// generic-field   = field-name ":" [ field-value ] NL
	// field-name      = token
	// field-value     = *( field-content | LWSP )
	// field-content   = *( token | separator | quoted-string )
	header := &r.header
	header.zero()
	header.kind = kindHeader
	header.place = placeInput // all received headers are in r.input
	// r.pFore is at headers (if any) or end of headers (if none).
	for { // each header
		// End of headers?
		if b := r.input[r.pFore]; b == '\r' {
			// Skip '\r'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
				return false
			}
			if r.input[r.pFore] != '\n' {
				r.headResult, r.failReason = StatusBadRequest, "bad end of headers"
				return false
			}
			break
		} else if b == '\n' {
			break
		}

		// field-name = token
		// token = 1*tchar

		r.pBack = r.pFore // now r.pBack is at header-field
		for {
			b := r.input[r.pFore]
			if t := webTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.input[r.pFore] = b
			} else if t == 3 { // '_'
				// For fcgi, do nothing
			} else if b == ':' {
				break
			} else {
				r.headResult, r.failReason = StatusBadRequest, "header name contains bad character"
				return false
			}
			header.hash += uint16(b)
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
				return false
			}
		}
		if nameSize := r.pFore - r.pBack; nameSize > 0 && nameSize <= 255 {
			header.nameFrom, header.nameSize = r.pBack, uint8(nameSize)
		} else {
			r.headResult, r.failReason = StatusBadRequest, "header name out of range"
			return false
		}
		// Skip ':'
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
			return false
		}
		// Skip OWS before field-value (and OWS after field-value if it is empty)
		for r.input[r.pFore] == ' ' || r.input[r.pFore] == '\t' {
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
				return false
			}
		}
		// field-value = *( field-content | LWSP )
		r.pBack = r.pFore // now r.pBack is at field-value (if not empty) or EOL (if field-value is empty)
		for {
			if b := r.input[r.pFore]; (b >= 0x20 && b != 0x7F) || b == 0x09 {
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
					return false
				}
				if r.input[r.pFore] != '\n' {
					r.headResult, r.failReason = StatusBadRequest, "header value contains bad eol"
					return false
				}
				break
			} else if b == '\n' {
				break
			} else {
				r.headResult, r.failReason = StatusBadRequest, "header value contains bad character"
				return false
			}
		}
		// r.pFore is at '\n'
		fore := r.pFore
		if r.input[fore-1] == '\r' {
			fore--
		}
		if fore > r.pBack { // field-value is not empty. now trim OWS after field-value
			for r.input[fore-1] == ' ' || r.input[fore-1] == '\t' {
				fore--
			}
		}
		header.value.set(r.pBack, fore)

		// Header is received in general algorithm. Now add it
		if !r.addPrime(header) {
			// r.headResult is set.
			return false
		}

		// Header is successfully received. Skip '\n'
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead() {
			return false
		}
		// r.pFore is now at the next header or end of headers.
		header.hash, header.flags = 0, 0 // reset for next header
	}
	r.receiving = webSectionContent
	// Skip end of headers
	r.pFore++
	// Now the head is received, and r.pFore is at the beginning of content (if exists).
	r.head.set(0, r.pFore)

	return true
}
func (r *fcgiResponse) addPrime(prime *pair) bool {
	if len(r.primes) == cap(r.primes) { // full
		if cap(r.primes) != cap(r.stockPrimes) { // too many primes
			r.headResult, r.failReason = StatusRequestHeaderFieldsTooLarge, "too many primes"
			return false
		}
		r.primes = getPairs()
		r.primes = append(r.primes, r.stockPrimes[:]...)
	}
	r.primes = append(r.primes, *prime)
	return true
}

func (r *fcgiResponse) Status() int16 { return r.status }

func (r *fcgiResponse) ContentSize() int64 { return -2 }   // fcgi is vague by default. we trust in framing protocol
func (r *fcgiResponse) IsVague() bool      { return true } // fcgi is vague by default. we trust in framing protocol

func (r *fcgiResponse) examineHead() bool {
	for i := 1; i < len(r.primes); i++ { // r.primes[0] is not used
		if !r.applyHeader(i) {
			// r.headResult is set.
			return false
		}
	}
	// content length is not known at this time, can't check.
	return true
}
func (r *fcgiResponse) applyHeader(index int) bool {
	header := &r.primes[index]
	headerName := header.nameAt(r.input)
	if sh := &fcgiResponseSingletonHeaderTable[fcgiResponseSingletonHeaderFind(header.hash)]; sh.hash == header.hash && bytes.Equal(sh.name, headerName) {
		header.setSingleton()
		if !sh.parse { // unnecessary to parse
			header.setParsed()
		} else if !r._parseHeader(header, &sh.desc, true) {
			// r.headResult is set.
			return false
		}
		if !sh.check(r, header, index) {
			// r.headResult is set.
			return false
		}
	} else if mh := &fcgiResponseImportantHeaderTable[fcgiResponseImportantHeaderFind(header.hash)]; mh.hash == header.hash && bytes.Equal(mh.name, headerName) {
		extraFrom := len(r.extras)
		if !r._splitHeader(header, &mh.desc) {
			// r.headResult is set.
			return false
		}
		if header.isCommaValue() { // has sub headers, check them
			if extraEdge := len(r.extras); !mh.check(r, r.extras, extraFrom, extraEdge) {
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

func (r *fcgiResponse) _parseHeader(header *pair, fdesc *desc, fully bool) bool { // data and params
	// TODO
	// use r._addExtra
	return true
}
func (r *fcgiResponse) _splitHeader(header *pair, fdesc *desc) bool {
	// TODO
	// use r._addExtra
	return true
}
func (r *fcgiResponse) _addExtra(extra *pair) bool {
	if len(r.extras) == cap(r.extras) { // full
		if cap(r.extras) != cap(r.stockExtras) { // too many extras
			r.headResult, r.failReason = StatusRequestHeaderFieldsTooLarge, "too many extras"
			return false
		}
		r.extras = getPairs()
		r.extras = append(r.extras, r.stockExtras[:]...)
	}
	r.extras = append(r.extras, *extra)
	r.hasExtra[extra.kind] = true
	return true
}

var ( // perfect hash table for singleton response headers
	fcgiResponseSingletonHeaderTable = [4]struct {
		parse bool // need general parse or not
		desc       // allowQuote, allowEmpty, allowParam, hasComment
		check func(*fcgiResponse, *pair, int) bool
	}{ // content-length content-type location status
		0: {false, desc{fcgiHashStatus, false, false, false, false, fcgiBytesStatus}, (*fcgiResponse).checkStatus},
		1: {false, desc{hashContentLength, false, false, false, false, bytesContentLength}, (*fcgiResponse).checkContentLength},
		2: {true, desc{hashContentType, false, false, true, false, bytesContentType}, (*fcgiResponse).checkContentType},
		3: {false, desc{hashLocation, false, false, false, false, bytesLocation}, (*fcgiResponse).checkLocation},
	}
	fcgiResponseSingletonHeaderFind = func(hash uint16) int { return (2704 / int(hash)) % 4 }
)

func (r *fcgiResponse) checkContentLength(header *pair, index int) bool {
	header.zero() // we don't believe the value provided by fcgi application. we believe fcgi framing protocol
	return true
}
func (r *fcgiResponse) checkContentType(header *pair, index int) bool {
	if r.indexes.contentType == 0 && !header.dataEmpty() {
		r.indexes.contentType = uint8(index)
		return true
	}
	r.headResult, r.failReason = StatusBadRequest, "bad or too many content-type"
	return false
}
func (r *fcgiResponse) checkStatus(header *pair, index int) bool {
	if value := header.valueAt(r.input); len(value) >= 3 {
		if status, ok := decToI64(value[0:3]); ok {
			r.status = int16(status)
			return true
		}
	}
	r.headResult, r.failReason = StatusBadRequest, "bad status"
	return false
}
func (r *fcgiResponse) checkLocation(header *pair, index int) bool {
	r.indexes.location = uint8(index)
	return true
}

var ( // perfect hash table for important response headers
	fcgiResponseImportantHeaderTable = [3]struct {
		desc  // allowQuote, allowEmpty, allowParam, hasComment
		check func(*fcgiResponse, []pair, int, int) bool
	}{ // connection transfer-encoding upgrade
		0: {desc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*fcgiResponse).checkTransferEncoding}, // deliberately false
		1: {desc{hashConnection, false, false, false, false, bytesConnection}, (*fcgiResponse).checkConnection},
		2: {desc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*fcgiResponse).checkUpgrade},
	}
	fcgiResponseImportantHeaderFind = func(hash uint16) int { return (1488 / int(hash)) % 3 }
)

func (r *fcgiResponse) checkConnection(pairs []pair, from int, edge int) bool { // Connection = #connection-option
	return r._delHeaders(pairs, from, edge)
}
func (r *fcgiResponse) checkTransferEncoding(pairs []pair, from int, edge int) bool { // Transfer-Encoding = #transfer-coding
	return r._delHeaders(pairs, from, edge)
}
func (r *fcgiResponse) checkUpgrade(pairs []pair, from int, edge int) bool { // Upgrade = #protocol
	return r._delHeaders(pairs, from, edge)
}
func (r *fcgiResponse) _delHeaders(pairs []pair, from int, edge int) bool {
	for i := from; i < edge; i++ {
		pairs[i].zero()
	}
	return true
}

func (r *fcgiResponse) cleanInput() {
	if r.hasContent() {
		r.imme.set(r.pFore, r.inputEdge)
		// We don't know the size of vague content. Let content receiver to decide & clean r.input.
	} else if _, _, err := r.fcgiRecvStdout(); err != io.EOF { // no content. must receive an endRequest
		r.headResult, r.failReason = StatusBadRequest, "bad endRequest"
	}
}

func (r *fcgiResponse) hasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified) responses do not include content.
	if r.status < StatusOK || r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might be of zero length.
	return true
}
func (r *fcgiResponse) takeContent() any { // to tempFile since we don't know the size of vague content
	switch content := r.recvContent().(type) {
	case tempFile: // [0, r.maxContentSize]
		r.contentFile = content.(*os.File)
		return r.contentFile
	case error: // i/o error or unexpected EOF
		// TODO: log err?
		if Debug() >= 2 {
			Println(content.Error())
		}
	}
	r.exchan.markBroken()
	return nil
}
func (r *fcgiResponse) recvContent() any { // to tempFile
	contentFile, err := r._newTempFile()
	if err != nil {
		return err
	}
	var p []byte
	for {
		p, err = r.readContent()
		if len(p) > 0 {
			if _, e := contentFile.Write(p); e != nil {
				err = e
				goto badRead
			}
		}
		if err == io.EOF {
			break
		} else if err != nil {
			goto badRead
		}
	}
	if _, err = contentFile.Seek(0, 0); err != nil {
		goto badRead
	}
	return contentFile // the tempFile
badRead:
	contentFile.Close()
	os.Remove(contentFile.Name())
	return err
}
func (r *fcgiResponse) readContent() (p []byte, err error) {
	if r.imme.notEmpty() {
		p, err = r.input[r.imme.from:r.imme.edge], nil
		r.imme.zero()
		return
	}
	if r.stdoutFrom != r.stdoutEdge {
		p, err = r.records[r.stdoutFrom:r.stdoutEdge], nil
		r.stdoutFrom, r.stdoutEdge = 0, 0
		return
	}
	if from, edge, err := r.fcgiRecvStdout(); from != edge {
		return r.records[from:edge], nil
	} else {
		return nil, err
	}
}

func (r *fcgiResponse) HasTrailers() bool { return false } // fcgi doesn't support trailers
func (r *fcgiResponse) examineTail() bool { return true }  // fcgi doesn't support trailers

func (r *fcgiResponse) delHopHeaders()  {} // for fcgi, nothing to delete
func (r *fcgiResponse) delHopTrailers() {} // fcgi doesn't support trailers

func (r *fcgiResponse) forHeaders(callback func(header *pair, name []byte, value []byte) bool) bool { // by Response.copyHeadFrom(). excluding sub headers
	for i := 1; i < len(r.primes); i++ { // r.primes[0] is not used
		if header := &r.primes[i]; header.hash != 0 && !header.isSubField() {
			if !callback(header, header.nameAt(r.input), header.valueAt(r.input)) {
				return false
			}
		}
	}
	return true
}
func (r *fcgiResponse) forTrailers(callback func(trailer *pair, name []byte, value []byte) bool) bool { // fcgi doesn't support trailers
	return true
}

func (r *fcgiResponse) arrayCopy(p []byte) bool { return true } // not used, but required by webIn interface

func (r *fcgiResponse) saveContentFilesDir() string { return r.exchan.relay.SaveContentFilesDir() }

func (r *fcgiResponse) _newTempFile() (tempFile, error) { // to save content to
	filesDir := r.saveContentFilesDir()
	filePath := r.exchan.unsafeMake(len(filesDir) + 19) // 19 bytes is enough for an int64
	n := copy(filePath, filesDir)
	n += r.exchan.makeTempName(filePath[n:], r.recvTime.Unix())
	return os.OpenFile(risky.WeakString(filePath[:n]), os.O_RDWR|os.O_CREATE, 0644)
}
func (r *fcgiResponse) _beforeRead(toTime *time.Time) error {
	now := time.Now()
	if toTime.IsZero() {
		*toTime = now
	}
	return r.exchan.setReadDeadline(now.Add(r.exchan.relay.backend.ReadTimeout()))
}
func (r *fcgiResponse) _tooSlow() bool {
	return r.recvTimeout > 0 && time.Now().Sub(r.bodyTime) >= r.recvTimeout
}

func (r *fcgiResponse) fcgiRecvStdout() (int32, int32, error) { // r.records[from:edge] is the stdout data.
	const (
		fcgiKindStdout     = 6 // [S] many (ends with an emptyStdout record)
		fcgiKindStderr     = 7 // [S] many (ends with an emptyStderr record)
		fcgiKindEndRequest = 3 // [D] only one
	)
recv:
	kind, from, edge, err := r.fcgiRecvRecord()
	if err != nil {
		return 0, 0, err
	}
	if kind == fcgiKindStdout && edge > from { // fast path
		return from, edge, nil
	}
	if kind == fcgiKindStderr {
		if edge > from && Debug() >= 2 {
			Printf("fcgi stderr=[%s]\n", r.records[from:edge])
		}
		goto recv
	}
	switch kind {
	case fcgiKindStdout: // must be emptyStdout
		for { // receive until endRequest
			kind, from, edge, err = r.fcgiRecvRecord()
			if kind == fcgiKindEndRequest {
				return 0, 0, io.EOF
			}
			// Only stderr records are allowed here.
			if kind != fcgiKindStderr {
				return 0, 0, fcgiReadBadRecord
			}
			// Must be stderr.
			if edge > from && Debug() >= 2 {
				Printf("fcgi stderr=[%s]\n", r.records[from:edge])
			}
		}
	case fcgiKindEndRequest:
		return 0, 0, io.EOF
	default: // unknown record
		return 0, 0, fcgiReadBadRecord
	}
}
func (r *fcgiResponse) fcgiRecvRecord() (kind byte, from int32, edge int32, err error) { // r.records[from:edge] is the record payload.
	remainSize := r.recordsEdge - r.recordsFrom
	// At least an fcgi header must be immediate
	if remainSize < fcgiHeaderSize {
		if n, e := r.fcgiGrowRecords(fcgiHeaderSize - int(remainSize)); e == nil {
			remainSize += int32(n)
		} else {
			err = e
			return
		}
	}
	// FCGI header is now immediate.
	payloadLen := int32(r.records[r.recordsFrom+4])<<8 + int32(r.records[r.recordsFrom+5])
	paddingLen := int32(r.records[r.recordsFrom+6])
	recordSize := fcgiHeaderSize + payloadLen + paddingLen // with padding
	// Is the whole record immediate?
	if recordSize > remainSize { // no, we need to make it immediate by reading the missing bytes
		// Shoud we switch to a larger r.records?
		if recordSize > int32(cap(r.records)) { // yes, because this record is too large
			records := getFCGIRecords()
			r.fcgiMoveRecords(records)
			r.records = records
		}
		// Now r.records is large enough to place this record, we can read the missing bytes of this record
		if n, e := r.fcgiGrowRecords(int(recordSize - remainSize)); e == nil {
			remainSize += int32(n)
		} else {
			err = e
			return
		}
	}
	// Now recordSize <= remainSize, the record is immediate, so continue parsing it.
	kind = r.records[r.recordsFrom+1]
	from = r.recordsFrom + fcgiHeaderSize
	edge = from + payloadLen // payload edge, ignoring padding
	if Debug() >= 2 {
		Printf("fcgiRecvRecord: kind=%d from=%d edge=%d payload=[%s] paddingLen=%d\n", kind, from, edge, r.records[from:edge], paddingLen)
	}
	// Clean up positions for next call.
	if recordSize == remainSize { // all remain data are consumed, so reset positions
		r.recordsFrom, r.recordsEdge = 0, 0
	} else { // recordSize < remainSize, extra records exist, so mark it for next call
		r.recordsFrom += recordSize
	}
	return
}
func (r *fcgiResponse) fcgiGrowRecords(size int) (int, error) { // r.records is large enough.
	// Should we slide to get enough space to grow?
	if size > cap(r.records)-int(r.recordsEdge) { // yes
		r.fcgiMoveRecords(r.records)
	}
	// We now have enough space to grow.
	n, err := r.exchan.readAtLeast(r.records[r.recordsEdge:], size)
	if err != nil { // since we *HAVE* to grow, short read is considered as an error
		return 0, err
	}
	r.recordsEdge += int32(n)
	return n, nil
}
func (r *fcgiResponse) fcgiMoveRecords(records []byte) { // so we can get more space to grow
	if r.recordsFrom > 0 {
		copy(records, r.records[r.recordsFrom:r.recordsEdge])
		r.recordsEdge -= r.recordsFrom
		r.recordsFrom = 0
	}
}

var ( // fcgi response errors
	fcgiReadBadRecord = errors.New("fcgi: bad record")
)

//////////////////////////////////////// FCGI protocol elements ////////////////////////////////////////

// FCGI Record = FCGI Header(8) + payload[65535] + padding[255]
// FCGI Header = version(1) + type(1) + requestId(2) + payloadLen(2) + paddingLen(1) + reserved(1)

// Discrete records are standalone.
// Streamed records end with an empty record (payloadLen=0).

const ( // fcgi constants
	fcgiHeaderSize = 8
	fcgiMaxPayload = 65535
	fcgiMaxPadding = 255
)

// poolFCGIRecords
var poolFCGIRecords sync.Pool

const fcgiMaxRecords = fcgiHeaderSize + fcgiMaxPayload + fcgiMaxPadding

func getFCGIRecords() []byte {
	if x := poolFCGIRecords.Get(); x == nil {
		return make([]byte, fcgiMaxRecords)
	} else {
		return x.([]byte)
	}
}
func putFCGIRecords(records []byte) {
	if cap(records) != fcgiMaxRecords {
		BugExitln("fcgi: bad records")
	}
	poolFCGIRecords.Put(records)
}

var ( // request records
	fcgiBeginKeepConn = []byte{ // 16 bytes
		1, 1, // version, FCGI_BEGIN_REQUEST
		0, 1, // request id = 1. we don't support pipelining or multiplex, only one request at a time, so request id is always 1. same below
		0, 8, // payload length = 8
		0, 0, // padding length = 0, reserved = 0
		0, 1, 1, 0, 0, 0, 0, 0, // role=responder, flags=keepConn
	}
	fcgiBeginDontKeep = []byte{ // 16 bytes
		1, 1, // version, FCGI_BEGIN_REQUEST
		0, 1, // request id = 1
		0, 8, // payload length = 8
		0, 0, // padding length = 0, reserved = 0
		0, 1, 0, 0, 0, 0, 0, 0, // role=responder, flags=dontKeep
	}
	fcgiEmptyParams = []byte{ // 8 bytes
		1, 4, // version, FCGI_PARAMS
		0, 1, // request id = 1
		0, 0, // payload length = 0
		0, 0, // padding length = 0, reserved = 0
	} // end of params
	fcgiEmptyStdin = []byte{ // 8 bytes
		1, 5, // version, FCGI_STDIN
		0, 1, // request id = 1
		0, 0, // payload length = 0
		0, 0, // padding length = 0, reserved = 0
	} // end of stdins
)

var ( // request param names
	fcgiBytesAuthType         = []byte("AUTH_TYPE")
	fcgiBytesContentLength    = []byte("CONTENT_LENGTH")
	fcgiBytesContentType      = []byte("CONTENT_TYPE")
	fcgiBytesDocumentRoot     = []byte("DOCUMENT_ROOT")
	fcgiBytesDocumentURI      = []byte("DOCUMENT_URI")
	fcgiBytesGatewayInterface = []byte("GATEWAY_INTERFACE")
	fcgiBytesHTTP_            = []byte("HTTP_") // prefix
	fcgiBytesHTTPS            = []byte("HTTPS")
	fcgiBytesPathInfo         = []byte("PATH_INFO")
	fcgiBytesPathTranslated   = []byte("PATH_TRANSLATED")
	fcgiBytesQueryString      = []byte("QUERY_STRING")
	fcgiBytesRedirectStatus   = []byte("REDIRECT_STATUS")
	fcgiBytesRemoteAddr       = []byte("REMOTE_ADDR")
	fcgiBytesRemoteHost       = []byte("REMOTE_HOST")
	fcgiBytesRequestMethod    = []byte("REQUEST_METHOD")
	fcgiBytesRequestScheme    = []byte("REQUEST_SCHEME")
	fcgiBytesRequestURI       = []byte("REQUEST_URI")
	fcgiBytesScriptFilename   = []byte("SCRIPT_FILENAME")
	fcgiBytesScriptName       = []byte("SCRIPT_NAME")
	fcgiBytesServerAddr       = []byte("SERVER_ADDR")
	fcgiBytesServerName       = []byte("SERVER_NAME")
	fcgiBytesServerPort       = []byte("SERVER_PORT")
	fcgiBytesServerProtocol   = []byte("SERVER_PROTOCOL")
	fcgiBytesServerSoftware   = []byte("SERVER_SOFTWARE")
)

var ( // request param values
	fcgiBytesCGI1_1 = []byte("CGI/1.1")
	fcgiBytesON     = []byte("on")
	fcgiBytes200    = []byte("200")
)

const ( // response header hashes
	fcgiHashStatus = 676
)

var ( // response header names
	fcgiBytesStatus = []byte("status")
)
