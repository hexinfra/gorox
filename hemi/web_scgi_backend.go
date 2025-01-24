// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// SCGI backend implementation. See: https://python.ca/scgi/protocol.txt

package hemi

import (
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("scgiBackend", func(compName string, stage *Stage) Backend {
		b := new(SCGIBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// SCGIBackend
type SCGIBackend struct {
	// Parent
	Backend_[*scgiNode]
	// States
}

func (b *SCGIBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
}

func (b *SCGIBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	b.ConfigureNodes()
}
func (b *SCGIBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	b.PrepareNodes()
}

func (b *SCGIBackend) CreateNode(compName string) Node {
	node := new(scgiNode)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

// scgiNode
type scgiNode struct {
	// Parent
	Node_[*SCGIBackend]
	// Mixins
	_contentSaver_ // so scgi responses can save their large contents in local file system.
	// States
}

func (n *scgiNode) onCreate(compName string, stage *Stage, backend *SCGIBackend) {
	n.Node_.OnCreate(compName, stage, backend)
}

func (n *scgiNode) OnConfigure() {
	n.Node_.OnConfigure()
	n._contentSaver_.onConfigure(n, 0*time.Second, 0*time.Second, TmpDir()+"/web/backends/"+n.backend.compName+"/"+n.compName)
}
func (n *scgiNode) OnPrepare() {
	n.Node_.OnPrepare()
	n._contentSaver_.onPrepare(n, 0755)
}

func (n *scgiNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s done\n", n.compName)
	}
	n.backend.DecSub() // node
}

func (n *scgiNode) dial() (*scgiExchan, error) {
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s dial %s\n", n.compName, n.address)
	}
	var (
		exchan *scgiExchan
		err    error
	)
	if n.UDSMode() {
		exchan, err = n._dialUDS()
	} else {
		exchan, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSubConn()
	return exchan, err
}
func (n *scgiNode) _dialUDS() (*scgiExchan, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getSCGIExchan(connID, n, netConn, rawConn), nil
}
func (n *scgiNode) _dialTCP() (*scgiExchan, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getSCGIExchan(connID, n, netConn, rawConn), nil
}

// scgiExchan
type scgiExchan struct {
	// Assocs
	response scgiResponse // the scgi response
	request  scgiRequest  // the scgi request
	// Exchan states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	id      int64           // the exchan id
	node    *scgiNode       // the node to which the exchan belongs
	region  Region          // a region-based memory pool
	netConn net.Conn        // *net.TCPConn or *net.UnixConn
	rawConn syscall.RawConn // for syscall
	// Exchan states (zeros)
	lastWrite time.Time // deadline of last write operation
	lastRead  time.Time // deadline of last read operation
}

var poolSCGIExchan sync.Pool

func getSCGIExchan(id int64, node *scgiNode, netConn net.Conn, rawConn syscall.RawConn) *scgiExchan {
	var exchan *scgiExchan
	if x := poolSCGIExchan.Get(); x == nil {
		exchan = new(scgiExchan)
		resp, req := &exchan.response, &exchan.request
		resp.exchan = exchan
		req.exchan = exchan
		req.response = resp
	} else {
		exchan = x.(*scgiExchan)
	}
	exchan.onUse(id, node, netConn, rawConn)
	return exchan
}
func putSCGIExchan(exchan *scgiExchan) {
	exchan.onEnd()
	poolSCGIExchan.Put(exchan)
}

func (x *scgiExchan) onUse(id int64, node *scgiNode, netConn net.Conn, rawConn syscall.RawConn) {
	x.id = id
	x.node = node
	x.region.Init()
	x.netConn = netConn
	x.rawConn = rawConn
	x.response.onUse()
	x.request.onUse()
}
func (x *scgiExchan) onEnd() {
	x.request.onEnd()
	x.response.onEnd()
	x.node = nil
	x.region.Free()
	x.netConn = nil
	x.rawConn = nil
}

func (x *scgiExchan) MakeTempName(dst []byte, unixTime int64) int {
	return makeTempName(dst, x.node.Stage().ID(), unixTime, x.id, 0)
}

func (x *scgiExchan) setReadDeadline() error {
	if deadline := time.Now().Add(x.node.readTimeout); deadline.Sub(x.lastRead) >= time.Second {
		if err := x.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		x.lastRead = deadline
	}
	return nil
}
func (x *scgiExchan) setWriteDeadline() error {
	if deadline := time.Now().Add(x.node.writeTimeout); deadline.Sub(x.lastWrite) >= time.Second {
		if err := x.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		x.lastWrite = deadline
	}
	return nil
}

func (x *scgiExchan) read(dst []byte) (int, error) { return x.netConn.Read(dst) }
func (x *scgiExchan) readAtLeast(dst []byte, min int) (int, error) {
	return io.ReadAtLeast(x.netConn, dst, min)
}
func (x *scgiExchan) write(src []byte) (int, error)             { return x.netConn.Write(src) }
func (x *scgiExchan) writev(srcVec *net.Buffers) (int64, error) { return srcVec.WriteTo(x.netConn) }

func (x *scgiExchan) buffer256() []byte          { return x.stockBuffer[:] }
func (x *scgiExchan) unsafeMake(size int) []byte { return x.region.Make(size) }

func (x *scgiExchan) Close() error {
	netConn := x.netConn
	putSCGIExchan(x)
	return netConn.Close()
}

// scgiResponse must implements the BackendResponse interface.
type scgiResponse struct { // incoming. needs parsing
	// Assocs
	exchan *scgiExchan
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
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
	_scgiResponse0           // all values in this struct must be zero by default!
}
type _scgiResponse0 struct { // for fast reset, entirely
	elemBack        int32 // element begins from. for parsing header elements
	elemFore        int32 // element spanning to. for parsing header elements
	head            span  // for debugging
	imme            span  // immediate bytes in r.input that belongs to content, not headers
	receiving       int8  // currently receiving. see httpSectionXXX
	contentTextKind int8  // kind of current r.contentText. see httpContentTextXXX
	receivedSize    int64 // bytes of currently received content
}

func (r *scgiResponse) onUse() {
	// TODO
}
func (r *scgiResponse) onEnd() {
	// TODO
	r._scgiResponse0 = _scgiResponse0{}
}

func (r *scgiResponse) reuse() {
	r.onEnd()
	r.onUse()
}

func (r *scgiResponse) KeepAlive() bool { return false } // required by the BackendResponse interface. actually scgi does not support persistent connections or keep-alive negotiations so this is not used

func (r *scgiResponse) HeadResult() int16 { return r.headResult }
func (r *scgiResponse) BodyResult() int16 { return r.bodyResult }

func (r *scgiResponse) recvHead() {
}

func (r *scgiResponse) Status() int16 { return r.status }

func (r *scgiResponse) ContentSize() int64 { return -1 }   // TODO
func (r *scgiResponse) IsVague() bool      { return true } // scgi response is vague by default

func (r *scgiResponse) examineHead() {
}

// scgiRequest
type scgiRequest struct { // outgoing. needs building
	// Assocs
	exchan   *scgiExchan
	response *scgiResponse
	// Exchan states (stocks)
	// Exchan states (controlled)
	// Exchan states (non-zeros)
	sendTimeout time.Duration // timeout to send the whole request. zero means no timeout
	// Exchan states (zeros)
	sendTime      time.Time   // the time when first write operation is performed
	vector        net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after exchan
	fixedVector   [7][]byte   // for sending request. reset after exchan. 120B
	_scgiRequest0             // all values in this struct must be zero by default!
}
type _scgiRequest0 struct { // for fast reset, entirely
	forbidContent bool // forbid content?
	forbidFraming bool // forbid content-length and transfer-encoding?
}

func (r *scgiRequest) onUse() {
	// TODO
}
func (r *scgiRequest) onEnd() {
	// TODO
	r._scgiRequest0 = _scgiRequest0{}
}

func (r *scgiRequest) proxyCopyHeaders(httpReq ServerRequest, proxyConfig *SCGIExchanProxyConfig) bool {
	// TODO
	return false
}
