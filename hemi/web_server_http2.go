// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/2 server implementation. See RFC 9113 and RFC 7541.

package hemi

import (
	"encoding/binary"
	"net"
	"sync"
	"syscall"
)

// server2Conn is the server-side HTTP/2 connection.
type server2Conn struct {
	// Parent
	http2Conn_[*server2Stream]
	// Mixins
	_serverConn_[*httpxGate]
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
	_server2Conn0 // all values in this struct must be zero by default!
}
type _server2Conn0 struct { // for fast reset, entirely
	waitReceive bool // ...
	//unackedSettings?
	//queuedControlFrames?
}

var poolServer2Conn sync.Pool

func getServer2Conn(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) *server2Conn {
	var servConn *server2Conn
	if x := poolServer2Conn.Get(); x == nil {
		servConn = new(server2Conn)
	} else {
		servConn = x.(*server2Conn)
	}
	servConn.onGet(id, gate, netConn, rawConn)
	return servConn
}
func putServer2Conn(servConn *server2Conn) {
	servConn.onPut()
	poolServer2Conn.Put(servConn)
}

func (c *server2Conn) onGet(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.http2Conn_.onGet(id, gate, netConn, rawConn)
	c._serverConn_.onGet(gate)
}
func (c *server2Conn) onPut() {
	c._server2Conn0 = _server2Conn0{}

	c._serverConn_.onPut()
	c.http2Conn_.onPut()
}

var server2PrefaceAndMore = []byte{
	// server preface settings
	0, 0, 30, // length=30
	4,          // kind=http2FrameSettings
	0,          // flags=
	0, 0, 0, 0, // streamID=0
	0, 1, 0x00, 0x00, 0x10, 0x00, // headerTableSize=4K
	0, 3, 0x00, 0x00, 0x00, 0x7f, // maxConcurrentStreams=127
	0, 4, 0x00, 0x00, 0xff, 0xff, // initialWindowSize=64K1
	0, 5, 0x00, 0x00, 0x40, 0x00, // maxFrameSize=16K
	0, 6, 0x00, 0x00, 0x40, 0x00, // maxHeaderListSize=16K

	// window update for the entire connection
	0, 0, 4, // length=4
	8,          // kind=http2FrameWindowUpdate
	0,          // flags=
	0, 0, 0, 0, // streamID=0
	0x7f, 0xff, 0x00, 0x00, // windowSize=2G1-64K1

	// ack client settings
	0, 0, 0, // length=0
	4,          // kind=http2FrameSettings
	1,          // flags=ack
	0, 0, 0, 0, // streamID=0
}

func (c *server2Conn) manage() { // runner
	Printf("========================== conn=%d start =========================\n", c.id)
	defer func() {
		Printf("========================== conn=%d exit =========================\n", c.id)
		putServer2Conn(c)
	}()
	go c.receive(true)
	if prism := <-c.incomingChan; prism != nil {
		c.closeConn()
		return
	}
	preface := <-c.incomingChan
	inFrame, ok := preface.(*http2InFrame)
	if !ok {
		c.closeConn()
		return
	}
	if inFrame.kind != http2FrameSettings || inFrame.ack {
		goto bad
	}
	if err := c._updatePeerSettings(inFrame, false); err != nil {
		goto bad
	}
	// Send server connection preface
	if err := c.setWriteDeadline(); err != nil {
		goto bad
	}
	if n, err := c.write(server2PrefaceAndMore); err == nil {
		Printf("--------------------- conn=%d CALL WRITE=%d -----------------------\n", c.id, n)
		Printf("conn=%d ---> %v\n", c.id, server2PrefaceAndMore)
		// Successfully handshake means we have acknowledged client settings and sent our settings. Still need to receive a settings ACK from client.
		goto serve
	} else {
		Printf("conn=%d error=%s\n", c.id, err.Error())
	}
bad:
	c.closeConn()
	c.waitReceive = true
	goto wait
serve:
	for { // each inFrame from c.receive() and outFrame from server streams
		select {
		case incoming := <-c.incomingChan: // got an incoming frame from c.receive()
			if inFrame, ok := incoming.(*http2InFrame); ok { // DATA, FIELDS, PRIORITY, RESET_STREAM, SETTINGS, PING, WINDOW_UPDATE, and unknown
				if inFrame.isUnknown() {
					// Implementations MUST ignore and discard frames of unknown types.
					continue
				}
				if err := server2InFrameProcessors[inFrame.kind](c, inFrame); err == nil {
					// Successfully processed. Next one.
					continue
				} else if h2e, ok := err.(http2Error); ok {
					c.goawayCloseConn(h2e)
				} else { // processor i/o error
					c.goawayCloseConn(http2ErrorInternal)
				}
				// c.manage() was broken, but c.receive() was not. need wait
				c.waitReceive = true
			} else { // got an error from c.receive(), it must be broken and quit.
				if h2e, ok := incoming.(http2Error); ok {
					c.goawayCloseConn(h2e)
				} else if netErr, ok := incoming.(net.Error); ok && netErr.Timeout() {
					c.goawayCloseConn(http2ErrorNoError)
				} else {
					c.closeConn()
				}
			}
			break serve
		case outFrame := <-c.outgoingChan: // got an outgoing frame from streams. MUST be fields frame or data frame!
			// TODO: collect as many outgoing frames as we can?
			Printf("%+v\n", outFrame)
			if outFrame.endStream { // a stream has ended
				c.retireStream(outFrame.stream)
				c.concurrentStreams--
			}
			if err := c.sendOutFrame(outFrame); err != nil {
				// send side is broken.
				c.closeConn()
				c.waitReceive = true
				break serve
			}
		}
	}
	Printf("conn=%d waiting for active streams to end\n", c.id)
	for c.concurrentStreams > 0 {
		outFrame := <-c.outgoingChan
		if outFrame.endStream {
			c.retireStream(outFrame.stream)
			c.concurrentStreams--
		}
	}
wait:
	if c.waitReceive {
		Printf("conn=%d waiting for c.receive() to quit\n", c.id)
		for {
			incoming := <-c.incomingChan
			if _, ok := incoming.(*http2InFrame); !ok { // an error from c.receive() means it's quit
				break
			}
		}
	}
	Printf("conn=%d c.manage() quit\n", c.id)
}

var server2InFrameProcessors = [http2NumFrameKinds]func(*server2Conn, *http2InFrame) error{
	(*server2Conn).processDataInFrame,
	(*server2Conn).processFieldsInFrame,
	(*server2Conn).processPriorityInFrame,
	(*server2Conn).processResetStreamInFrame,
	(*server2Conn).processSettingsInFrame,
	(*server2Conn).processPushPromiseInFrame,
	(*server2Conn).processPingInFrame,
	(*server2Conn).processGoawayInFrame,
	(*server2Conn).processWindowUpdateInFrame,
	(*server2Conn).processContinuationInFrame,
}

func (c *server2Conn) processFieldsInFrame(fieldsInFrame *http2InFrame) error {
	var (
		servStream *server2Stream
		servReq    *server2Request
	)
	streamID := fieldsInFrame.streamID
	if streamID > c.lastStreamID { // new stream
		if c.concurrentStreams == http2MaxConcurrentStreams {
			return http2ErrorProtocol
		}
		c.lastStreamID = streamID
		c.cumulativeStreams.Add(1)
		servStream = getServer2Stream(c, streamID, c.peerSettings.initialWindowSize)
		servReq = &servStream.request
		Println("xxxxxxxxxxx")
		if !c.hpackDecode(fieldsInFrame.effective(), servReq.joinHeaders) {
			Println("yyyyyyyyyyy")
			putServer2Stream(servStream)
			return http2ErrorCompression
		}
		Println("zzzzzzzz")
		if fieldsInFrame.endStream {
			servStream.state = http2StateRemoteClosed
		} else {
			servStream.state = http2StateOpen
		}
		c.appendStream(servStream)
		c.concurrentStreams++
		go servStream.execute()
	} else { // old stream
		servStream := c.searchStream(streamID)
		if servStream == nil { // no specified active stream
			return http2ErrorProtocol
		}
		if servStream.state != http2StateOpen {
			return http2ErrorProtocol
		}
		if !fieldsInFrame.endStream { // here must be trailer fields that end the stream
			return http2ErrorProtocol
		}
		servReq = &servStream.request
		servReq.receiving = httpSectionTrailers
		if !c.hpackDecode(fieldsInFrame.effective(), servReq.joinTrailers) {
			return http2ErrorCompression
		}
	}
	return nil
}
func (c *server2Conn) processSettingsInFrame(settingsInFrame *http2InFrame) error {
	if settingsInFrame.ack {
		c.acknowledged = true
		return nil
	}
	// TODO: client sent a new settings
	return nil
}

func (c *server2Conn) goawayCloseConn(h2e http2Error) {
	goawayOutFrame := &c.outFrame
	goawayOutFrame.streamID = 0
	goawayOutFrame.length = 8
	goawayOutFrame.kind = http2FrameGoaway
	binary.BigEndian.PutUint32(goawayOutFrame.outBuffer[0:4], c.lastStreamID)
	binary.BigEndian.PutUint32(goawayOutFrame.outBuffer[4:8], uint32(h2e))
	goawayOutFrame.payload = goawayOutFrame.outBuffer[0:8]
	c.sendOutFrame(goawayOutFrame) // ignore error
	goawayOutFrame.zero()
	c.closeConn()
}

func (c *server2Conn) closeConn() {
	if DebugLevel() >= 2 {
		Printf("conn=%d closed by manage()\n", c.id)
	}
	c.netConn.Close()
	c.gate.DecConcurrentConns()
	c.gate.DecConn()
}

// server2Stream is the server-side HTTP/2 stream.
type server2Stream struct {
	// Parent
	http2Stream_[*server2Conn]
	// Mixins
	_serverStream_
	// Assocs
	request  server2Request  // the http/2 request.
	response server2Response // the http/2 response.
	socket   *server2Socket  // ...
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
	_server2Stream0 // all values in this struct must be zero by default!
}
type _server2Stream0 struct { // for fast reset, entirely
}

var poolServer2Stream sync.Pool

func getServer2Stream(conn *server2Conn, id uint32, remoteWindow int32) *server2Stream {
	var servStream *server2Stream
	if x := poolServer2Stream.Get(); x == nil {
		servStream = new(server2Stream)
		servReq, servResp := &servStream.request, &servStream.response
		servReq.stream = servStream
		servReq.in = servReq
		servResp.stream = servStream
		servResp.out = servResp
		servResp.request = servReq
	} else {
		servStream = x.(*server2Stream)
	}
	servStream.onUse(conn, id, remoteWindow)
	return servStream
}
func putServer2Stream(servStream *server2Stream) {
	servStream.onEnd()
	poolServer2Stream.Put(servStream)
}

func (s *server2Stream) onUse(conn *server2Conn, id uint32, remoteWindow int32) { // for non-zeros
	s.http2Stream_.onUse(conn, id)
	s._serverStream_.onUse()

	s.localWindow = _64K1         // max size of r.bodyWindow
	s.remoteWindow = remoteWindow // may be changed by the peer
	s.request.onUse()
	s.response.onUse()
}
func (s *server2Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}
	s._server2Stream0 = _server2Stream0{}

	s._serverStream_.onEnd()
	s.http2Stream_.onEnd()
}

func (s *server2Stream) execute() { // runner
	defer putServer2Stream(s)
	// TODO ...
	if DebugLevel() >= 2 {
		Println("stream processing...")
	}
}
func (s *server2Stream) _serveAbnormal(req *server2Request, resp *server2Response) { // 4xx & 5xx
	// TODO
	// s.setWriteDeadline() // for _serveAbnormal
	// s.writev()
}
func (s *server2Stream) _writeContinue() bool { // 100 continue
	// TODO
	// s.setWriteDeadline() // for _writeContinue
	// s.write()
	return false
}

func (s *server2Stream) executeExchan(webapp *Webapp, req *server2Request, resp *server2Response) { // request & response
	// TODO
	webapp.dispatchExchan(req, resp)
}
func (s *server2Stream) executeSocket() { // see RFC 8441: https://datatracker.ietf.org/doc/html/rfc8441
	// TODO
}

// server2Request is the server-side HTTP/2 request.
type server2Request struct { // incoming. needs parsing
	// Parent
	serverRequest_
	// Assocs
	in2 _http2In_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server2Request) onUse() {
	r.serverRequest_.onUse(Version2)
	r.in2.onUse(&r._httpIn_)
}
func (r *server2Request) onEnd() {
	r.serverRequest_.onEnd()
	r.in2.onEnd()
}

func (r *server2Request) joinHeaders(p []byte) bool {
	if len(p) > 0 {
		if !r.in2._growHeaders(int32(len(p))) {
			return false
		}
		r.inputEdge += int32(copy(r.input[r.inputEdge:], p))
	}
	return true
}
func (r *server2Request) readContent() (data []byte, err error) { return r.in2.readContent() }
func (r *server2Request) joinTrailers(p []byte) bool {
	// TODO: to r.array
	return false
}

// server2Response is the server-side HTTP/2 response.
type server2Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Assocs
	out2 _http2Out_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server2Response) onUse() {
	r.serverResponse_.onUse(Version2)
	r.out2.onUse(&r._httpOut_)
}
func (r *server2Response) onEnd() {
	r.serverResponse_.onEnd()
	r.out2.onEnd()
}

func (r *server2Response) addHeader(name []byte, value []byte) bool {
	return r.out2.addHeader(name, value)
}
func (r *server2Response) header(name []byte) (value []byte, ok bool) { return r.out2.header(name) }
func (r *server2Response) hasHeader(name []byte) bool                 { return r.out2.hasHeader(name) }
func (r *server2Response) delHeader(name []byte) (deleted bool)       { return r.out2.delHeader(name) }
func (r *server2Response) delHeaderAt(i uint8)                        { r.out2.delHeaderAt(i) }

func (r *server2Response) AddHTTPSRedirection(authority string) bool {
	// TODO
	return false
}
func (r *server2Response) AddHostnameRedirection(hostname string) bool {
	// TODO
	return false
}
func (r *server2Response) AddDirectoryRedirection() bool {
	// TODO
	return false
}

func (r *server2Response) AddCookie(cookie *Cookie) bool {
	// TODO
	return false
}

func (r *server2Response) sendChain() error { return r.out2.sendChain() }

func (r *server2Response) echoHeaders() error { return r.out2.writeHeaders() }
func (r *server2Response) echoChain() error   { return r.out2.echoChain() }

func (r *server2Response) addTrailer(name []byte, value []byte) bool {
	return r.out2.addTrailer(name, value)
}
func (r *server2Response) trailer(name []byte) (value []byte, ok bool) { return r.out2.trailer(name) }

func (r *server2Response) proxyPass1xx(backResp BackendResponse) bool {
	backResp.proxyDelHopHeaderFields()
	r.status = backResp.Status()
	if !backResp.proxyWalkHeaderLines(r, func(out httpOut, headerLine *pair, headerName []byte, lineValue []byte) bool {
		return out.insertHeader(headerLine.nameHash, headerName, lineValue) // some header fields (e.g. "connection") are restricted
	}) {
		return false
	}
	// TODO
	// For next use.
	r.onEnd()
	r.onUse()
	return false
}
func (r *server2Response) proxyPassHeaders() error          { return r.out2.writeHeaders() }
func (r *server2Response) proxyPassBytes(data []byte) error { return r.out2.proxyPassBytes(data) }

func (r *server2Response) finalizeHeaders() { // add at most 256 bytes
	// TODO
	/*
		// date: Sun, 06 Nov 1994 08:49:37 GMT
		if r.iDate == 0 {
			clock := r.stream.(*server2Stream).conn.gate.stage.clock
			r.outputEdge += uint16(clock.writeDate2(r.output[r.outputEdge:]))
		}
	*/
}
func (r *server2Response) finalizeVague() error {
	// TODO
	return nil
}

func (r *server2Response) addedHeaders() []byte { return nil } // TODO
func (r *server2Response) fixedHeaders() []byte { return nil } // TODO

// server2Socket is the server-side HTTP/2 webSocket.
type server2Socket struct { // incoming and outgoing
	// Parent
	serverSocket_
	// Assocs
	so2 _http2Socket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolServer2Socket sync.Pool

func getServer2Socket(stream *server2Stream) *server2Socket {
	// TODO
	return nil
}
func putServer2Socket(socket *server2Socket) {
	// TODO
}

func (s *server2Socket) onUse() {
	s.serverSocket_.onUse()
	s.so2.onUse(&s._httpSocket_)
}
func (s *server2Socket) onEnd() {
	s.serverSocket_.onEnd()
	s.so2.onEnd()
}

func (s *server2Socket) serverTodo2() {
	s.serverTodo()
	s.so2.todo2()
}
