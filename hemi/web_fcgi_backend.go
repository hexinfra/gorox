// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// FCGI backend implementation. See: https://fastcgi-archives.github.io/FastCGI_Specification.html

package hemi

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("fcgiBackend", func(compName string, stage *Stage) Backend {
		b := new(FCGIBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// FCGIBackend is a FCGI backend.
type FCGIBackend struct {
	// Parent
	Backend_[*fcgiNode]
	// States
}

func (b *FCGIBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
}

func (b *FCGIBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	b.ConfigureNodes()
}
func (b *FCGIBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	b.PrepareNodes()
}

func (b *FCGIBackend) CreateNode(compName string) Node {
	node := new(fcgiNode)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *FCGIBackend) FetchExchan(req ServerRequest) (*fcgiExchan, error) {
	return b.nodes[b.nodeIndexGet()].fetchExchan()
}
func (b *FCGIBackend) StoreExchan(exchan *fcgiExchan) {
	exchan.conn.node.storeExchan(exchan)
}

// fcgiNode is a node in FCGI backend.
type fcgiNode struct {
	// Parent
	Node_[*FCGIBackend]
	// Mixins
	_contentSaver_ // so fcgi responses can save their large contents in local file system.
	// States
	idleTimeout                 time.Duration // conn idle timeout
	maxLifetime                 time.Duration // conn's max lifetime
	keepConn                    bool          // instructs FCGI server to keep conn?
	keepAliveConns              int32         // max conns to keep alive. requires keepConn to be true
	maxCumulativeExchansPerConn int32         // max exchans of one conn. 0 means infinite
	connPool                    struct {      // free list of conns in this node
		sync.Mutex
		head *fcgiConn
		tail *fcgiConn
		qnty int // size of the list
	}
}

func (n *fcgiNode) onCreate(compName string, stage *Stage, backend *FCGIBackend) {
	n.Node_.OnCreate(compName, stage, backend)
}

func (n *fcgiNode) OnConfigure() {
	n.Node_.OnConfigure()
	n._contentSaver_.onConfigure(n, 0*time.Second, 0*time.Second, TmpDir()+"/web/backends/"+n.backend.compName+"/"+n.compName)

	// .idleTimeout
	n.ConfigureDuration("idleTimeout", &n.idleTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".idleTimeout has an invalid value")
	}, 2*time.Second)

	// .maxLifetime
	n.ConfigureDuration("maxLifetime", &n.maxLifetime, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".maxLifetime has an invalid value")
	}, 1*time.Minute)

	// .keepConn
	n.ConfigureBool("keepConn", &n.keepConn, true)

	if n.keepConn {
		// .keepAliveConns
		n.ConfigureInt32("keepAliveConns", &n.keepAliveConns, func(value int32) error {
			if value > 0 {
				return nil
			}
			return errors.New("bad keepAliveConns in node")
		}, 10)

		// .maxCumulativeExchansPerConn
		n.ConfigureInt32("maxCumulativeExchansPerConn", &n.maxCumulativeExchansPerConn, func(value int32) error {
			if value >= 0 {
				return nil
			}
			return errors.New(".maxCumulativeExchansPerConn has an invalid value")
		}, 1000)
	}
}
func (n *fcgiNode) OnPrepare() {
	n.Node_.OnPrepare()
	n._contentSaver_.onPrepare(n, 0755)
}

func (n *fcgiNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	if size := n.closeFree(); size > 0 {
		n.DecSubConns(size)
	}
	n.WaitSubConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("fcgiNode=%s done\n", n.compName)
	}
	n.backend.DecSub() // node
}

func (n *fcgiNode) fetchExchan() (*fcgiExchan, error) {
	if n.keepConn {
		fcgiConn := n.pullConn()
		nodeDown := n.isDown()
		if fcgiConn != nil {
			if !nodeDown && fcgiConn.isAlive() && fcgiConn.cumulativeExchans.Add(1) <= n.maxCumulativeExchansPerConn {
				return fcgiConn.fetchExchan()
			}
			fcgiConn.Close()
			n.DecSubConn()
		}
		if nodeDown {
			return nil, errNodeDown
		}
	}
	fcgiConn, err := n.dial()
	if err != nil {
		return nil, errNodeDown
	}
	return fcgiConn.fetchExchan()
}
func (n *fcgiNode) dial() (*fcgiConn, error) {
	if DebugLevel() >= 2 {
		Printf("fcgiNode=%s dial %s\n", n.compName, n.address)
	}
	var (
		conn *fcgiConn
		err  error
	)
	if n.UDSMode() {
		conn, err = n._dialUDS()
	} else {
		conn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSubConn()
	return conn, err
}
func (n *fcgiNode) _dialUDS() (*fcgiConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("fcgiNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getFCGIConn(connID, n, netConn, rawConn), nil
}
func (n *fcgiNode) _dialTCP() (*fcgiConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("fcgiNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getFCGIConn(connID, n, netConn, rawConn), nil
}
func (n *fcgiNode) storeExchan(exchan *fcgiExchan) {
	fcgiConn := exchan.conn
	fcgiConn.storeExchan(exchan)

	if fcgiConn.isBroken() || n.isDown() || !fcgiConn.isAlive() || !n.keepConn {
		fcgiConn.Close()
		n.DecSubConn()
	} else {
		n.pushConn(fcgiConn)
	}
}

func (n *fcgiNode) pullConn() *fcgiConn {
	list := &n.connPool

	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		return nil
	}
	conn := list.head
	list.head = conn.next
	conn.next = nil
	list.qnty--

	return conn
}
func (n *fcgiNode) pushConn(conn *fcgiConn) {
	list := &n.connPool

	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		list.head = conn
		list.tail = conn
	} else { // >= 1
		list.tail.next = conn
		list.tail = conn
	}
	list.qnty++
}
func (n *fcgiNode) closeFree() int {
	list := &n.connPool

	list.Lock()
	defer list.Unlock()

	conn := list.head
	for conn != nil {
		next := conn.next
		conn.next = nil
		conn.Close()
		conn = next
	}
	qnty := list.qnty
	list.qnty = 0
	list.head, list.tail = nil, nil

	return qnty
}

// fcgiConn is a connection to a FCGI node.
type fcgiConn struct {
	// Assocs
	next   *fcgiConn  // the linked-list
	exchan fcgiExchan // an fcgi connection has exactly one stream. currently we don't support multiplex
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	id         int64           // the conn id
	node       *fcgiNode       // the node to which the connection belongs
	netConn    net.Conn        // *net.TCPConn or *net.UnixConn
	rawConn    syscall.RawConn // for syscall
	expireTime time.Time       // when the conn is considered expired
	// Conn states (zeros)
	cumulativeExchans atomic.Int32 // how many exchans have been used?
	broken            atomic.Bool  // is conn broken?
	counter           atomic.Int64 // can be used to generate a random number
	lastWrite         time.Time    // deadline of last write operation
	lastRead          time.Time    // deadline of last read operation
}

var poolFCGIConn sync.Pool

func getFCGIConn(id int64, node *fcgiNode, netConn net.Conn, rawConn syscall.RawConn) *fcgiConn {
	var conn *fcgiConn
	if x := poolFCGIConn.Get(); x == nil {
		conn = new(fcgiConn)
		exchan := &conn.exchan
		exchan.conn = conn
		resp, req := &exchan.response, &exchan.request
		resp.exchan = exchan
		req.exchan = exchan
		req.response = resp
	} else {
		conn = x.(*fcgiConn)
	}
	conn.onGet(id, node, netConn, rawConn)
	return conn
}
func putFCGIConn(conn *fcgiConn) {
	conn.onPut()
	poolFCGIConn.Put(conn)
}

func (c *fcgiConn) onGet(id int64, node *fcgiNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
	c.expireTime = time.Now().Add(node.idleTimeout)
}
func (c *fcgiConn) onPut() {
	c.node = nil
	c.netConn = nil
	c.rawConn = nil
	c.expireTime = time.Time{}
	c.cumulativeExchans.Store(0)
	c.broken.Store(false)
	c.counter.Store(0)
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *fcgiConn) MakeTempName(dst []byte, unixTime int64) int {
	return makeTempName(dst, c.node.Stage().ID(), unixTime, c.id, c.counter.Add(1))
}

func (c *fcgiConn) isAlive() bool { return time.Now().Before(c.expireTime) }

func (c *fcgiConn) fetchExchan() (*fcgiExchan, error) {
	exchan := &c.exchan
	exchan.onUse()
	return exchan, nil
}
func (c *fcgiConn) storeExchan(exchan *fcgiExchan) {
	exchan.onEnd()
}

func (c *fcgiConn) markBroken()    { c.broken.Store(true) }
func (c *fcgiConn) isBroken() bool { return c.broken.Load() }

func (c *fcgiConn) setReadDeadline() error {
	if deadline := time.Now().Add(c.node.readTimeout); deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *fcgiConn) setWriteDeadline() error {
	if deadline := time.Now().Add(c.node.writeTimeout); deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *fcgiConn) read(dst []byte) (int, error) { return c.netConn.Read(dst) }
func (c *fcgiConn) readAtLeast(dst []byte, min int) (int, error) {
	return io.ReadAtLeast(c.netConn, dst, min)
}
func (c *fcgiConn) write(src []byte) (int, error)             { return c.netConn.Write(src) }
func (c *fcgiConn) writev(srcVec *net.Buffers) (int64, error) { return srcVec.WriteTo(c.netConn) }

func (c *fcgiConn) Close() error {
	netConn := c.netConn
	putFCGIConn(c)
	return netConn.Close()
}

// fcgiExchan is a request/response exchange in a FCGI connection.
type fcgiExchan struct {
	// Assocs
	conn     *fcgiConn    // the fcgi conn
	response fcgiResponse // the fcgi response
	request  fcgiRequest  // the fcgi request
	// Exchan states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	region Region // a region-based memory pool
	// Exchan states (zeros)
}

func (x *fcgiExchan) onUse() { // for non-zeros
	x.region.Init()
	x.response.onUse()
	x.request.onUse()
}
func (x *fcgiExchan) onEnd() { // for zeros
	x.request.onEnd()
	x.response.onEnd()
	x.region.Free()
}

func (x *fcgiExchan) markBroken()    { x.conn.markBroken() }
func (x *fcgiExchan) isBroken() bool { return x.conn.isBroken() }

func (x *fcgiExchan) setReadDeadline() error  { return x.conn.setReadDeadline() }
func (x *fcgiExchan) setWriteDeadline() error { return x.conn.setWriteDeadline() }

func (x *fcgiExchan) read(dst []byte) (int, error) { return x.conn.read(dst) }
func (x *fcgiExchan) readAtLeast(dst []byte, min int) (int, error) {
	return x.conn.readAtLeast(dst, min)
}
func (x *fcgiExchan) write(src []byte) (int, error)             { return x.conn.write(src) }
func (x *fcgiExchan) writev(srcVec *net.Buffers) (int64, error) { return x.conn.writev(srcVec) }

func (x *fcgiExchan) buffer256() []byte          { return x.stockBuffer[:] }
func (x *fcgiExchan) unsafeMake(size int) []byte { return x.region.Make(size) }

// fcgiResponse is the FCGI response in a FCGI exchange. It must implements the BackendResponse interface.
type fcgiResponse struct { // incoming. needs parsing
	// Assocs
	exchan *fcgiExchan
	// Exchan states (stocks)
	stockRecords [8456]byte // for r.records. fcgiHeaderSize + 8K + fcgiMaxPadding. good for PHP
	stockInput   [2048]byte // for r.input
	stockPrimes  [48]pair   // for r.primes
	stockExtras  [16]pair   // for r.extras
	// Exchan states (controlled)
	header pair // to overcome the limitation of Go's escape analysis when receiving headers
	// Exchan states (non-zeros)
	records        []byte        // bytes of incoming fcgi records. [<r.stockRecords>/fcgiMaxRecords]
	input          []byte        // bytes of incoming response headers. [<r.stockInput>/4K/16K]
	primes         []pair        // prime fcgi response headers
	extras         []pair        // extra fcgi response headers
	recvTimeout    time.Duration // timeout to recv the whole response content. zero means no timeout
	maxContentSize int64         // max content size allowed for current response
	status         int16         // 200, 302, 404, ...
	headResult     int16         // result of receiving response head. values are same as http status for convenience
	bodyResult     int16         // result of receiving response body. values are same as http status for convenience
	// Exchan states (zeros)
	failReason     string    // the reason of headResult or bodyResult
	bodyTime       time.Time // the time when first body read operation is performed on this exchan
	contentText    []byte    // if loadable, the received and loaded content of current response is at r.contentText[:r.receivedSize]
	contentFile    *os.File  // used by r.proxyTakeContent(), if content is tempFile. will be closed on exchan ends
	_fcgiResponse0           // all values in this struct must be zero by default!
}
type _fcgiResponse0 struct { // for fast reset, entirely
	recordsFrom     int32    // from position of current records
	recordsEdge     int32    // edge position of current records
	stdoutFrom      int32    // if stdout's payload is too large to be appended to r.input, use this to note current from position
	stdoutEdge      int32    // see above, to note current edge position
	elemBack        int32    // element begins from. for parsing header elements
	elemFore        int32    // element spanning to. for parsing header elements
	head            span     // for debugging
	imme            span     // immediate bytes in r.input that belongs to content, not headers
	hasExtra        [8]bool  // see pairXXX for indexes
	inputEdge       int32    // edge position of r.input
	receiving       int8     // currently receiving. see httpSectionXXX
	contentTextKind int8     // kind of current r.contentText. see httpContentTextXXX
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
	r.recvTimeout = r.exchan.conn.node.recvTimeout
	r.maxContentSize = r.exchan.conn.node.maxContentSize
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
	r.bodyTime = time.Time{}

	if r.contentTextKind == httpContentTextPool {
		PutNK(r.contentText)
	}
	r.contentText = nil // other content text kinds are only references, just reset.

	if r.contentFile != nil {
		r.contentFile.Close()
		if DebugLevel() >= 2 {
			Println("contentFile is left as is, not removed!")
		} else if err := os.Remove(r.contentFile.Name()); err != nil {
			// TODO: log?
		}
		r.contentFile = nil
	}

	r._fcgiResponse0 = _fcgiResponse0{}
}

func (r *fcgiResponse) reuse() {
	r.onEnd()
	r.onUse()
}

func (r *fcgiResponse) KeepAlive() bool { return r.exchan.conn.node.keepConn } // this exists to satisfy the BackendResponse interface but is not actually used, as fcgi lacks the mechanism to negotiate keep-alive or not

func (r *fcgiResponse) HeadResult() int16 { return r.headResult }
func (r *fcgiResponse) BodyResult() int16 { return r.bodyResult }

func (r *fcgiResponse) recvHead() {
	// The entire response head must be received within one read timeout
	if err := r.exchan.setReadDeadline(); err != nil {
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
	r.tidyInput()
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
	header.kind = pairHeader
	header.place = placeInput // all received headers are in r.input
	// r.elemFore is at headers (if any) or end of headers (if none).
	for { // each header
		// End of headers?
		if b := r.input[r.elemFore]; b == '\r' {
			// Skip '\r'
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
				return false
			}
			if r.input[r.elemFore] != '\n' {
				r.headResult, r.failReason = StatusBadRequest, "bad end of headers"
				return false
			}
			break
		} else if b == '\n' {
			break
		}

		// field-name = token
		// token = 1*tchar

		r.elemBack = r.elemFore // now r.elemBack is at header-field
		for {
			b := r.input[r.elemFore]
			if t := httpTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.input[r.elemFore] = b
			} else if t == 3 { // '_'
				// For fcgi, do nothing
			} else if b == ':' {
				break
			} else {
				r.headResult, r.failReason = StatusBadRequest, "header name contains bad character"
				return false
			}
			header.nameHash += uint16(b)
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
				return false
			}
		}
		if nameSize := r.elemFore - r.elemBack; nameSize > 0 && nameSize <= 255 {
			header.nameFrom, header.nameSize = r.elemBack, uint8(nameSize)
		} else {
			r.headResult, r.failReason = StatusBadRequest, "header name out of range"
			return false
		}
		// Skip ':'
		if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
			return false
		}
		// Skip OWS before field-value (and OWS after field-value if it is empty)
		for r.input[r.elemFore] == ' ' || r.input[r.elemFore] == '\t' {
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
				return false
			}
		}
		// field-value = *( field-content | LWSP )
		r.elemBack = r.elemFore // now r.elemBack is at field-value (if not empty) or EOL (if field-value is empty)
		for {
			if b := r.input[r.elemFore]; (b >= 0x20 && b != 0x7F) || b == 0x09 {
				if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
					return false
				}
				if r.input[r.elemFore] != '\n' {
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
		// r.elemFore is at '\n'
		fore := r.elemFore
		if r.input[fore-1] == '\r' {
			fore--
		}
		if fore > r.elemBack { // field-value is not empty. now trim OWS after field-value
			for r.input[fore-1] == ' ' || r.input[fore-1] == '\t' {
				fore--
			}
		}
		header.value.set(r.elemBack, fore)

		// Header is received in general algorithm. Now add it
		if !r.addPrime(header) {
			// r.headResult is set.
			return false
		}

		// Header is successfully received. Skip '\n'
		if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead() {
			return false
		}
		// r.elemFore is now at the next header or end of headers.
		header.nameHash, header.flags = 0, 0 // reset for next header
	}
	r.receiving = httpSectionContent
	// Skip end of headers
	r.elemFore++
	// Now the head is received, and r.elemFore is at the beginning of content (if exists).
	r.head.set(0, r.elemFore)

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
	if sh := &fcgiResponseSingletonHeaderTable[fcgiResponseSingletonHeaderFind(header.nameHash)]; sh.nameHash == header.nameHash && bytes.Equal(sh.name, headerName) {
		header.setSingleton()
		if !sh.parse { // unnecessary to parse generally
			header.setParsed()
		} else if !r._parseHeader(header, &sh.fdesc, true) {
			// r.headResult is set.
			return false
		}
		if !sh.check(r, header, index) {
			// r.headResult is set.
			return false
		}
	} else if mh := &fcgiResponseImportantHeaderTable[fcgiResponseImportantHeaderFind(header.nameHash)]; mh.nameHash == header.nameHash && bytes.Equal(mh.name, headerName) {
		extraFrom := len(r.extras)
		if !r._splitHeader(header, &mh.fdesc) {
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

func (r *fcgiResponse) _parseHeader(header *pair, fdesc *fdesc, fully bool) bool { // data and params
	// TODO
	// use r._addExtra
	return true
}
func (r *fcgiResponse) _splitHeader(header *pair, fdesc *fdesc) bool {
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
		fdesc      // allowQuote, allowEmpty, allowParam, hasComment
		check func(*fcgiResponse, *pair, int) bool
	}{ // content-length content-type location status
		0: {false, fdesc{fcgiHashStatus, false, false, false, false, fcgiBytesStatus}, (*fcgiResponse).checkStatus},
		1: {false, fdesc{hashContentLength, false, false, false, false, bytesContentLength}, (*fcgiResponse).checkContentLength},
		2: {true, fdesc{hashContentType, false, false, true, false, bytesContentType}, (*fcgiResponse).checkContentType},
		3: {false, fdesc{hashLocation, false, false, false, false, bytesLocation}, (*fcgiResponse).checkLocation},
	}
	fcgiResponseSingletonHeaderFind = func(nameHash uint16) int { return (2704 / int(nameHash)) % len(fcgiResponseSingletonHeaderTable) }
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
		fdesc // allowQuote, allowEmpty, allowParam, hasComment
		check func(*fcgiResponse, []pair, int, int) bool
	}{ // connection transfer-encoding upgrade
		0: {fdesc{hashTransferEncoding, false, false, false, false, bytesTransferEncoding}, (*fcgiResponse).checkTransferEncoding}, // deliberately false
		1: {fdesc{hashConnection, false, false, false, false, bytesConnection}, (*fcgiResponse).checkConnection},
		2: {fdesc{hashUpgrade, false, false, false, false, bytesUpgrade}, (*fcgiResponse).checkUpgrade},
	}
	fcgiResponseImportantHeaderFind = func(nameHash uint16) int { return (1488 / int(nameHash)) % len(fcgiResponseImportantHeaderTable) }
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

func (r *fcgiResponse) tidyInput() {
	if r.HasContent() {
		r.imme.set(r.elemFore, r.inputEdge)
		// We don't know the size of vague content. Let content receiver to decide & clean r.input.
	} else if _, _, err := r.fcgiRecvStdout(); err != io.EOF { // no content. must receive an endRequest
		r.headResult, r.failReason = StatusBadRequest, "bad endRequest"
	}
}

func (r *fcgiResponse) HasContent() bool {
	// All 1xx (Informational), 204 (No Content), and 304 (Not Modified) responses do not include content.
	if r.status < StatusOK || r.status == StatusNoContent || r.status == StatusNotModified {
		return false
	}
	// All other responses do include content, although that content might be of zero length.
	return true
}
func (r *fcgiResponse) proxyTakeContent() any { // to tempFile since we don't know the size of vague content
	switch content := r._recvContent().(type) {
	case tempFile: // [0, r.maxContentSize]
		r.contentFile = content.(*os.File)
		return r.contentFile
	case error: // i/o error or unexpected EOF
		// TODO: log err?
		if DebugLevel() >= 2 {
			Println(content.Error())
		}
	}
	r.exchan.markBroken()
	return nil
}
func (r *fcgiResponse) _recvContent() any { // to tempFile
	contentFile, err := r._newTempFile()
	if err != nil {
		return err
	}
	var data []byte
	for {
		data, err = r.readContent()
		if len(data) > 0 {
			if _, e := contentFile.Write(data); e != nil {
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
func (r *fcgiResponse) readContent() (data []byte, err error) {
	if r.imme.notEmpty() {
		data, err = r.input[r.imme.from:r.imme.edge], nil
		r.imme.zero()
		return
	}
	if r.stdoutFrom != r.stdoutEdge {
		data, err = r.records[r.stdoutFrom:r.stdoutEdge], nil
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

func (r *fcgiResponse) proxyDelHopHeaders()  {} // for fcgi, nothing to delete
func (r *fcgiResponse) proxyDelHopTrailers() {} // fcgi doesn't support trailers

func (r *fcgiResponse) proxyWalkHeaders(callback func(header *pair, name []byte, value []byte) bool) bool { // excluding sub headers
	for i := 1; i < len(r.primes); i++ { // r.primes[0] is not used
		if header := &r.primes[i]; header.nameHash != 0 && !header.isSubField() {
			if !callback(header, header.nameAt(r.input), header.valueAt(r.input)) {
				return false
			}
		}
	}
	return true
}
func (r *fcgiResponse) proxyWalkTrailers(callback func(trailer *pair, name []byte, value []byte) bool) bool { // fcgi doesn't support trailers
	return true
}

func (r *fcgiResponse) saveContentFilesDir() string {
	return r.exchan.conn.node.SaveContentFilesDir()
}

func (r *fcgiResponse) _newTempFile() (tempFile, error) { // to save content to
	filesDir := r.saveContentFilesDir()
	filePath := r.exchan.unsafeMake(len(filesDir) + 19) // 19 bytes is enough for an int64
	n := copy(filePath, filesDir)
	n += r.exchan.conn.MakeTempName(filePath[n:], time.Now().Unix())
	return os.OpenFile(WeakString(filePath[:n]), os.O_RDWR|os.O_CREATE, 0644)
}

func (r *fcgiResponse) _isLongTime() bool {
	return r.recvTimeout > 0 && time.Since(r.bodyTime) >= r.recvTimeout
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
		if edge > from && DebugLevel() >= 2 {
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
			if edge > from && DebugLevel() >= 2 {
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
	if DebugLevel() >= 2 {
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
	if r.bodyTime.IsZero() {
		r.bodyTime = time.Now()
	}
	if err := r.exchan.setReadDeadline(); err != nil {
		return 0, err
	}
	n, err := r.exchan.readAtLeast(r.records[r.recordsEdge:], size)
	if err == nil && r._isLongTime() {
		err = fcgiReadLongTime
	}
	if err != nil { // since we *HAVE* to grow size, short read is considered as an error
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
	fcgiReadLongTime  = errors.New("fcgi: read costs a long time")
)

// fcgiRequest is the FCGI request in a FCGI exchange.
type fcgiRequest struct { // outgoing. needs building
	// Assocs
	exchan   *fcgiExchan
	response *fcgiResponse
	// Exchan states (stocks)
	stockParams [2048]byte // for r.params
	// Exchan states (controlled)
	paramsHeader [8]byte // used by params record
	stdinHeader  [8]byte // used by stdin record
	// Exchan states (non-zeros)
	params      []byte        // place the payload of exactly one FCGI_PARAMS record. [<r.stockParams>/16K]
	sendTimeout time.Duration // timeout to send the whole request. zero means no timeout
	// Exchan states (zeros)
	sendTime      time.Time   // the time when first write operation is performed
	vector        net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after exchan
	fixedVector   [7][]byte   // for sending request. reset after exchan. 120B
	_fcgiRequest0             // all values in this struct must be zero by default!
}
type _fcgiRequest0 struct { // for fast reset, entirely
	paramsEdge    uint16 // edge of r.params. max size of r.params must be <= 16K.
	forbidContent bool   // forbid content?
	forbidFraming bool   // forbid content-length and transfer-encoding?
}

func (r *fcgiRequest) onUse() {
	copy(r.paramsHeader[:], fcgiEmptyParams) // payloadLen (r.paramsHeader[4:6]) needs modification on using
	copy(r.stdinHeader[:], fcgiEmptyStdin)   // payloadLen (r.stdinHeader[4:6]) needs modification for every stdin record on using
	r.params = r.stockParams[:]
	r.sendTimeout = r.exchan.conn.node.sendTimeout
}
func (r *fcgiRequest) onEnd() {
	if cap(r.params) != cap(r.stockParams) {
		PutNK(r.params)
		r.params = nil
	}
	r.sendTime = time.Time{}
	r.vector = nil
	r.fixedVector = [7][]byte{}

	r._fcgiRequest0 = _fcgiRequest0{}
}

func (r *fcgiRequest) proxyCopyHeaders(httpReq ServerRequest, proxyConfig *FCGIExchanProxyConfig) bool {
	// Add meta params
	if !r._addMetaParam(fcgiBytesGatewayInterface, fcgiBytesCGI1_1) { // GATEWAY_INTERFACE
		return false
	}
	if !r._addMetaParam(fcgiBytesServerSoftware, bytesGorox) { // SERVER_SOFTWARE
		return false
	}
	if !r._addMetaParam(fcgiBytesServerProtocol, httpReq.UnsafeVersion()) { // SERVER_PROTOCOL
		return false
	}
	if !r._addMetaParam(fcgiBytesRequestMethod, httpReq.UnsafeMethod()) { // REQUEST_METHOD
		return false
	}
	if !r._addMetaParam(fcgiBytesRequestScheme, httpReq.UnsafeScheme()) { // REQUEST_SCHEME
		return false
	}
	if !r._addMetaParam(fcgiBytesRequestURI, httpReq.UnsafeURI()) { // REQUEST_URI
		return false
	}
	if !r._addMetaParam(fcgiBytesServerName, httpReq.UnsafeHostname()) { // SERVER_NAME
		return false
	}
	if !r._addMetaParam(fcgiBytesRedirectStatus, fcgiBytes200) { // REDIRECT_STATUS
		return false
	}
	if httpReq.IsHTTPS() && !r._addMetaParam(fcgiBytesHTTPS, fcgiBytesON) { // HTTPS
		return false
	}
	scriptFilename := proxyConfig.ScriptFilename
	if len(scriptFilename) == 0 {
		absPath := httpReq.unsafeAbsPath()
		indexFile := proxyConfig.IndexFile
		if absPath[len(absPath)-1] == '/' && len(indexFile) > 0 {
			scriptFilename = httpReq.UnsafeMake(len(absPath) + len(indexFile))
			copy(scriptFilename, absPath)
			copy(scriptFilename[len(absPath):], indexFile)
		} else {
			scriptFilename = absPath
		}
	}
	if !r._addMetaParam(fcgiBytesScriptFilename, scriptFilename) { // SCRIPT_FILENAME
		return false
	}
	if !r._addMetaParam(fcgiBytesScriptName, httpReq.UnsafePath()) { // SCRIPT_NAME
		return false
	}
	var value []byte
	if value = httpReq.UnsafeQueryString(); len(value) > 1 {
		value = value[1:] // excluding '?'
	} else {
		value = nil
	}
	if !r._addMetaParam(fcgiBytesQueryString, value) { // QUERY_STRING
		return false
	}
	if value = httpReq.UnsafeContentLength(); value != nil && !r._addMetaParam(fcgiBytesContentLength, value) { // CONTENT_LENGTH
		return false
	}
	if value = httpReq.UnsafeContentType(); value != nil && !r._addMetaParam(fcgiBytesContentType, value) { // CONTENT_TYPE
		return false
	}

	// Add http params
	if !httpReq.proxyWalkHeaders(func(header *pair, name []byte, value []byte) bool {
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
	if header.isUnderscore() {
		// TODO: got a "foo_bar" header and user prefer it. avoid name conflicts with header which is like "foo-bar"
		return true
	} else {
		return r._addParam(name, value, true)
	}
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

func (r *fcgiRequest) proxyPassMessage(httpReq ServerRequest) error { // only for sized (>0) content. vague content must use proxyPostMessage(), as we don't use backend-side chunking
	r.vector = r.fixedVector[0:4]
	r._setBeginRequest(&r.vector[0])
	r.vector[1] = r.paramsHeader[:]
	r.vector[2] = r.params[:r.paramsEdge] // effective params
	r.vector[3] = fcgiEmptyParams
	if err := r._writeVector(); err != nil {
		return err
	}
	for {
		data, err := httpReq.readContent()
		if len(data) > 0 {
			size := len(data)
			r.stdinHeader[4], r.stdinHeader[5] = byte(size>>8), byte(size)
			if err == io.EOF { // EOF is immediate, write with emptyStdin
				r.vector = r.fixedVector[0:3]
				r.vector[0] = r.stdinHeader[:]
				r.vector[1] = data
				r.vector[2] = fcgiEmptyStdin
				return r._writeVector()
			}
			// EOF is not immediate, err must be nil.
			r.vector = r.fixedVector[0:2]
			r.vector[0] = r.stdinHeader[:]
			r.vector[1] = data
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
func (r *fcgiRequest) proxyPostMessage(httpContent any) error { // nil, []byte, and *os.File. for bufferClientContent or vague ServerRequest content
	if contentText, ok := httpContent.([]byte); ok { // text, <= 64K1
		return r.sendText(contentText)
	} else if contentFile, ok := httpContent.(*os.File); ok { // file
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

	for {
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

func (r *fcgiRequest) _setBeginRequest(beginRequest *[]byte) {
	if r.exchan.conn.node.keepConn {
		*beginRequest = fcgiBeginKeepConn
	} else {
		*beginRequest = fcgiBeginDontKeep
	}
}

func (r *fcgiRequest) _writeBytes(data []byte) error {
	if r.exchan.isBroken() {
		return fcgiWriteBroken
	}
	if len(data) == 0 {
		return nil
	}
	if r.sendTime.IsZero() {
		r.sendTime = time.Now()
	}
	if err := r.exchan.setWriteDeadline(); err != nil {
		r.exchan.markBroken()
		return err
	}
	_, err := r.exchan.write(data)
	return r._longTimeCheck(err)
}
func (r *fcgiRequest) _writeVector() error {
	if r.exchan.isBroken() {
		return fcgiWriteBroken
	}
	if len(r.vector) == 1 && len(r.vector[0]) == 0 {
		return nil
	}
	if r.sendTime.IsZero() {
		r.sendTime = time.Now()
	}
	if err := r.exchan.setWriteDeadline(); err != nil {
		r.exchan.markBroken()
		return err
	}
	_, err := r.exchan.writev(&r.vector)
	return r._longTimeCheck(err)
}
func (r *fcgiRequest) _longTimeCheck(err error) error {
	if err == nil && r._isLongTime() {
		err = fcgiWriteLongTime
	}
	if err != nil {
		r.exchan.markBroken()
	}
	return err
}
func (r *fcgiRequest) _isLongTime() bool {
	return r.sendTimeout > 0 && time.Since(r.sendTime) >= r.sendTimeout
}

var ( // fcgi request errors
	fcgiWriteLongTime = errors.New("fcgi: write costs a long time")
	fcgiWriteBroken   = errors.New("fcgi: write broken")
)
