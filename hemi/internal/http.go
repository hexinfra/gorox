// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General HTTP protocol elements, incoming message and outgoing message implementation.

package internal

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/risky"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"io"
	"net"
	"os"
	"time"
)

const ( // version codes. keep sync with ../hemi.go
	Version1_0 = 0 // must be 0
	Version1_1 = 1
	Version2   = 2
	Version3   = 3
)

var ( // version strings and byteses
	httpStringHTTP1_0  = "HTTP/1.0"
	httpStringHTTP1_1  = "HTTP/1.1"
	httpStringHTTP2    = "HTTP/2"
	httpStringHTTP3    = "HTTP/3"
	httpBytesHTTP1_0   = []byte(httpStringHTTP1_0)
	httpBytesHTTP1_1   = []byte(httpStringHTTP1_1)
	httpBytesHTTP2     = []byte(httpStringHTTP2)
	httpBytesHTTP3     = []byte(httpStringHTTP3)
	httpVersionStrings = [...]string{
		Version1_0: httpStringHTTP1_0,
		Version1_1: httpStringHTTP1_1,
		Version2:   httpStringHTTP2,
		Version3:   httpStringHTTP3,
	}
	httpVersionByteses = [...][]byte{
		Version1_0: httpBytesHTTP1_0,
		Version1_1: httpBytesHTTP1_1,
		Version2:   httpBytesHTTP2,
		Version3:   httpBytesHTTP3,
	}
)

const ( // scheme codes. keep sync with ../hemi.go
	SchemeHTTP  = 0 // must be 0
	SchemeHTTPS = 1
)

var ( // scheme strings and byteses
	httpStringHTTP    = "http"
	httpStringHTTPS   = "https"
	httpBytesHTTP     = []byte(httpStringHTTP)
	httpBytesHTTPS    = []byte(httpStringHTTPS)
	httpSchemeStrings = [...]string{
		SchemeHTTP:  httpStringHTTP,
		SchemeHTTPS: httpStringHTTPS,
	}
	httpSchemeByteses = [...][]byte{
		SchemeHTTP:  httpBytesHTTP,
		SchemeHTTPS: httpBytesHTTPS,
	}
)

const ( // method codes. keep sync with ../hemi.go
	MethodGET     = 0x00000001
	MethodHEAD    = 0x00000002
	MethodPOST    = 0x00000004
	MethodPUT     = 0x00000008
	MethodDELETE  = 0x00000010
	MethodCONNECT = 0x00000020
	MethodOPTIONS = 0x00000040
	MethodTRACE   = 0x00000080
	MethodPATCH   = 0x00000100
	MethodLINK    = 0x00000200
	MethodUNLINK  = 0x00000400
	MethodQUERY   = 0x00000800
)

var ( // method hash table
	httpMethodBytes = []byte("GET HEAD POST PUT DELETE CONNECT OPTIONS TRACE PATCH LINK UNLINK QUERY")
	httpMethodTable = [12]struct {
		hash uint16
		from uint8
		edge uint8
		code uint32
	}{
		0:  {326, 9, 13, MethodPOST},
		1:  {465, 58, 64, MethodUNLINK},
		2:  {302, 53, 57, MethodLINK},
		3:  {435, 18, 24, MethodDELETE},
		4:  {224, 0, 3, MethodGET},
		5:  {406, 65, 70, MethodQUERY},
		6:  {274, 4, 8, MethodHEAD},
		7:  {368, 47, 52, MethodPATCH},
		8:  {367, 41, 46, MethodTRACE},
		9:  {556, 33, 40, MethodOPTIONS},
		10: {522, 25, 32, MethodCONNECT},
		11: {249, 14, 17, MethodPUT},
	}
	httpMethodFind = func(hash uint16) int { return (11774 / int(hash)) % 12 }
)

const ( // status codes. keep sync with ../hemi.go
	// 1XX
	StatusContinue           = 100
	StatusSwitchingProtocols = 101
	StatusProcessing         = 102
	StatusEarlyHints         = 103
	// 2XX
	StatusOK                         = 200
	StatusCreated                    = 201
	StatusAccepted                   = 202
	StatusNonAuthoritativeInfomation = 203
	StatusNoContent                  = 204
	StatusResetContent               = 205
	StatusPartialContent             = 206
	StatusMultiStatus                = 207
	StatusAlreadyReported            = 208
	StatusIMUsed                     = 226
	// 3XX
	StatusMultipleChoices   = 300
	StatusMovedPermanently  = 301
	StatusFound             = 302
	StatusSeeOther          = 303
	StatusNotModified       = 304
	StatusUseProxy          = 305
	StatusTemporaryRedirect = 307
	StatusPermanentRedirect = 308
	// 4XX
	StatusBadRequest                  = 400
	StatusUnauthorized                = 401
	StatusPaymentRequired             = 402
	StatusForbidden                   = 403
	StatusNotFound                    = 404
	StatusMethodNotAllowed            = 405
	StatusNotAcceptable               = 406
	StatusProxyAuthenticationRequired = 407
	StatusRequestTimeout              = 408
	StatusConflict                    = 409
	StatusGone                        = 410
	StatusLengthRequired              = 411
	StatusPreconditionFailed          = 412
	StatusContentTooLarge             = 413
	StatusURITooLong                  = 414
	StatusUnsupportedMediaType        = 415
	StatusRangeNotSatisfiable         = 416
	StatusExpectationFailed           = 417
	StatusMisdirectedRequest          = 421
	StatusUnprocessableEntity         = 422
	StatusLocked                      = 423
	StatusFailedDependency            = 424
	StatusTooEarly                    = 425
	StatusUpgradeRequired             = 426
	StatusPreconditionRequired        = 428
	StatusTooManyRequests             = 429
	StatusRequestHeaderFieldsTooLarge = 431
	StatusUnavailableForLegalReasons  = 451
	// 5XX
	StatusInternalServerError           = 500
	StatusNotImplemented                = 501
	StatusBadGateway                    = 502
	StatusServiceUnavailable            = 503
	StatusGatewayTimeout                = 504
	StatusHTTPVersionNotSupported       = 505
	StatusVariantAlsoNegotiates         = 506
	StatusInsufficientStorage           = 507
	StatusLoopDetected                  = 508
	StatusNotExtended                   = 510
	StatusNetworkAuthenticationRequired = 511
)

const ( // misc http types
	httpModeNormal = 0 // request & response, must be 0
	httpModeTCPTun = 1 // CONNECT method
	httpModeUDPTun = 2 // upgrade: connect-udp
	httpModeSocket = 3 // upgrade: websocket

	httpTargetOrigin    = 0 // must be 0
	httpTargetAbsolute  = 1
	httpTargetAuthority = 2 // hostname:port
	httpTargetAsterisk  = 3 // *

	httpFormNotForm    = 0 // must be 0
	httpFormURLEncoded = 1
	httpFormMultipart  = 2

	httpCodingIdentity = 0 // must be 0
	httpCodingCompress = 1
	httpCodingDeflate  = 2
	httpCodingGzip     = 3
	httpCodingBrotli   = 4

	httpSectionControl  = 0 // must be 0
	httpSectionHeaders  = 1
	httpSectionContent  = 2
	httpSectionTrailers = 3
)

const ( // hashes of http fields. value is calculated by adding all ASCII values.
	// Pseudo headers
	httpHashAuthority = 1059 // :authority
	httpHashMethod    = 699  // :method
	httpHashPath      = 487  // :path
	httpHashScheme    = 687  // :scheme
	httpHashStatus    = 734  // :status
	// General fields
	httpHashCacheControl       = 1314
	httpHashConnection         = 1072
	httpHashContentDisposition = 2013
	httpHashContentEncoding    = 1647
	httpHashContentLanguage    = 1644
	httpHashContentLength      = 1450
	httpHashContentRange       = 1333
	httpHashContentType        = 1258
	httpHashKeepAlive          = 995
	httpHashPragma             = 632
	httpHashTE                 = 217
	httpHashTrailer            = 755
	httpHashTransferEncoding   = 1753
	httpHashUpgrade            = 744
	httpHashVia                = 320
	// Request fields
	httpHashAccept            = 624
	httpHashAcceptCharset     = 1415
	httpHashAcceptEncoding    = 1508
	httpHashAcceptLanguage    = 1505
	httpHashCookie            = 634
	httpHashExpect            = 649
	httpHashForwarded         = 958
	httpHashHost              = 446
	httpHashIfMatch           = 777
	httpHashIfModifiedSince   = 1660
	httpHashIfNoneMatch       = 1254
	httpHashIfRange           = 777
	httpHashIfUnmodifiedSince = 1887
	httpHashProxyConnection   = 1695
	httpHashRange             = 525
	httpHashUserAgent         = 1019
	// Response fields
	httpHashAcceptRanges      = 1309
	httpHashAllow             = 543
	httpHashDate              = 414
	httpHashETag              = 417
	httpHashExpires           = 768
	httpHashLastModified      = 1314
	httpHashLocation          = 857
	httpHashProxyAuthenticate = 1902
	httpHashServer            = 663
	httpHashSetCookie         = 1011
	httpHashVary              = 450
	httpHashWWWAuthenticate   = 1681
)

var ( // byteses of http fields.
	// Pseudo headers
	httpBytesAuthority = []byte(":authority")
	httpBytesMethod    = []byte(":method")
	httpBytesPath      = []byte(":path")
	httpBytesScheme    = []byte(":scheme")
	httpBytesStatus    = []byte(":status")
	// General fields
	httpBytesCacheControl       = []byte("cache-control")
	httpBytesConnection         = []byte("connection")
	httpBytesContentDisposition = []byte("content-disposition")
	httpBytesContentEncoding    = []byte("content-encoding")
	httpBytesContentLanguage    = []byte("content-language")
	httpBytesContentLength      = []byte("content-length")
	httpBytesContentRange       = []byte("content-range")
	httpBytesContentType        = []byte("content-type")
	httpBytesKeepAlive          = []byte("keep-alive")
	httpBytesPragma             = []byte("pragma")
	httpBytesTE                 = []byte("te")
	httpBytesTrailer            = []byte("trailer")
	httpBytesTransferEncoding   = []byte("transfer-encoding")
	httpBytesUpgrade            = []byte("upgrade")
	httpBytesVia                = []byte("via")
	// Request fields
	httpBytesAccept            = []byte("accept")
	httpBytesAcceptCharset     = []byte("accept-charset")
	httpBytesAcceptEncoding    = []byte("accept-encoding")
	httpBytesAcceptLanguage    = []byte("accept-language")
	httpBytesCookie            = []byte("cookie")
	httpBytesExpect            = []byte("expect")
	httpBytesForwarded         = []byte("forwarded")
	httpBytesHost              = []byte("host")
	httpBytesIfMatch           = []byte("if-match")
	httpBytesIfModifiedSince   = []byte("if-modified-since")
	httpBytesIfNoneMatch       = []byte("if-none-match")
	httpBytesIfRange           = []byte("if-range")
	httpBytesIfUnmodifiedSince = []byte("if-unmodified-since")
	httpBytesProxyConnection   = []byte("proxy-connection")
	httpBytesRange             = []byte("range")
	// Response fields
	httpBytesAcceptRanges    = []byte("accept-ranges")
	httpBytesAllow           = []byte("allow")
	httpBytesDate            = []byte("date")
	httpBytesETag            = []byte("etag")
	httpBytesExpires         = []byte("expires")
	httpBytesLastModified    = []byte("last-modified")
	httpBytesLocation        = []byte("location")
	httpBytesServer          = []byte("server")
	httpBytesSetCookie       = []byte("set-cookie")
	httpBytesVary            = []byte("vary")
	httpBytesWWWAuthenticate = []byte("www-authenticate")
)

var ( // misc http strings & byteses.
	// Strings
	httpStringColonPort80  = ":80"
	httpStringColonPort443 = ":443"
	httpStringSlash        = "/"
	httpStringAsterisk     = "*"
	// Byteses
	httpBytesColonPort80    = []byte(httpStringColonPort80)
	httpBytesColonPort443   = []byte(httpStringColonPort443)
	httpBytesSlash          = []byte(httpStringSlash)
	httpBytesAsterisk       = []byte(httpStringAsterisk)
	httpBytes100Continue    = []byte("100-continue")
	httpBytesBoundary       = []byte("boundary")
	httpBytesBytes          = []byte("bytes")
	httpBytesBytesEqual     = []byte("bytes=")
	httpBytesChunked        = []byte("chunked")
	httpBytesClose          = []byte("close")
	httpBytesColonSpace     = []byte(": ")
	httpBytesCompress       = []byte("compress")
	httpBytesCRLF           = []byte("\r\n")
	httpBytesDeflate        = []byte("deflate")
	httpBytesFilename       = []byte("filename")
	httpBytesFormData       = []byte("form-data")
	httpBytesGzip           = []byte("gzip")
	httpBytesBrotli         = []byte("br")
	httpBytesIdentity       = []byte("identity")
	httpBytesURLEncodedForm = []byte("application/x-www-form-urlencoded")
	httpBytesMultipartForm  = []byte("multipart/form-data")
	httpBytesName           = []byte("name")
	httpBytesNone           = []byte("none")
	httpBytesTextHTML       = []byte("text/html; charset=utf-8")
	httpBytesTrailers       = []byte("trailers")
	httpBytesWebSocket      = []byte("websocket")
)

var httpTchar = [256]int8{ // tchar = ALPHA / DIGIT / "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." / "^" / "_" / "`" / "|" / "~"
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 1, 0, 1, 1, 1, 1, 1, 0, 0, 1, 1, 0, 1, 1, 0, //   !   # $ % & '     * +   - .
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, // 0 1 2 3 4 5 6 7 8 9
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, //   A B C D E F G H I J K L M N O
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 1, 1, // P Q R S T U V W X Y Z       ^ _
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // ` a b c d e f g h i j k l m n o
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 0, 1, 0, // p q r s t u v w x y z   |   ~
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
var httpPchar = [256]int8{ // pchar = ALPHA / DIGIT / "!" / "$" / "&" / "'" / "(" / ")" / "*" / "+" / "," / "-" / "." / ":" / ";" / "=" / "@" / "_" / "~" / pct-encoded. '/' is pchar to improve performance.
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 1, 0, 0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, //   !     $   & ' ( ) * + , - . /
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 0, 0, // 0 1 2 3 4 5 6 7 8 9 : ;   =
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // @ A B C D E F G H I J K L M N O
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 1, // P Q R S T U V W X Y Z         _
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, //   a b c d e f g h i j k l m n o
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 1, 0, // p q r s t u v w x y z       ~
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
var httpVchar = [256]int8{ // for field-value: (b >= 0x20 && b != 0x7F) || b == 0x09
	0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
}
var httpKchar = [256]int8{ // cookie-octet = 0x21 / 0x23-0x2B / 0x2D-0x3A / 0x3C-0x5B / 0x5D-0x7E
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 1, 1, //   !   # $ % & ' ( ) * +   - . /
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 1, 1, 1, // 0 1 2 3 4 5 6 7 8 9 :   < = > ?
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // @ A B C D E F G H I J K L M N O
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 1, 1, // P Q R S T U V W X Y Z [   ] ^ _
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // ` a b c d e f g h i j k l m n o
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, // p q r s t u v w x y z { | } ~
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
var httpNchar = [256]int8{ // for hostname
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, //                           - .
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, // 0 1 2 3 4 5 6 7 8 9
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, //
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, //
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, //   a b c d e f g h i j k l m n o
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, // p q r s t u v w x y z
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

var httpHuffmanCodes = [256]uint32{ // 1K, for huffman encoding
	0x00001ff8, 0x007fffd8, 0x0fffffe2, 0x0fffffe3, 0x0fffffe4, 0x0fffffe5, 0x0fffffe6, 0x0fffffe7,
	0x0fffffe8, 0x00ffffea, 0x3ffffffc, 0x0fffffe9, 0x0fffffea, 0x3ffffffd, 0x0fffffeb, 0x0fffffec,
	0x0fffffed, 0x0fffffee, 0x0fffffef, 0x0ffffff0, 0x0ffffff1, 0x0ffffff2, 0x3ffffffe, 0x0ffffff3,
	0x0ffffff4, 0x0ffffff5, 0x0ffffff6, 0x0ffffff7, 0x0ffffff8, 0x0ffffff9, 0x0ffffffa, 0x0ffffffb,
	0x00000014, 0x000003f8, 0x000003f9, 0x00000ffa, 0x00001ff9, 0x00000015, 0x000000f8, 0x000007fa,
	0x000003fa, 0x000003fb, 0x000000f9, 0x000007fb, 0x000000fa, 0x00000016, 0x00000017, 0x00000018,
	0x00000000, 0x00000001, 0x00000002, 0x00000019, 0x0000001a, 0x0000001b, 0x0000001c, 0x0000001d,
	0x0000001e, 0x0000001f, 0x0000005c, 0x000000fb, 0x00007ffc, 0x00000020, 0x00000ffb, 0x000003fc,
	0x00001ffa, 0x00000021, 0x0000005d, 0x0000005e, 0x0000005f, 0x00000060, 0x00000061, 0x00000062,
	0x00000063, 0x00000064, 0x00000065, 0x00000066, 0x00000067, 0x00000068, 0x00000069, 0x0000006a,
	0x0000006b, 0x0000006c, 0x0000006d, 0x0000006e, 0x0000006f, 0x00000070, 0x00000071, 0x00000072,
	0x000000fc, 0x00000073, 0x000000fd, 0x00001ffb, 0x0007fff0, 0x00001ffc, 0x00003ffc, 0x00000022,
	0x00007ffd, 0x00000003, 0x00000023, 0x00000004, 0x00000024, 0x00000005, 0x00000025, 0x00000026,
	0x00000027, 0x00000006, 0x00000074, 0x00000075, 0x00000028, 0x00000029, 0x0000002a, 0x00000007,
	0x0000002b, 0x00000076, 0x0000002c, 0x00000008, 0x00000009, 0x0000002d, 0x00000077, 0x00000078,
	0x00000079, 0x0000007a, 0x0000007b, 0x00007ffe, 0x000007fc, 0x00003ffd, 0x00001ffd, 0x0ffffffc,
	0x000fffe6, 0x003fffd2, 0x000fffe7, 0x000fffe8, 0x003fffd3, 0x003fffd4, 0x003fffd5, 0x007fffd9,
	0x003fffd6, 0x007fffda, 0x007fffdb, 0x007fffdc, 0x007fffdd, 0x007fffde, 0x00ffffeb, 0x007fffdf,
	0x00ffffec, 0x00ffffed, 0x003fffd7, 0x007fffe0, 0x00ffffee, 0x007fffe1, 0x007fffe2, 0x007fffe3,
	0x007fffe4, 0x001fffdc, 0x003fffd8, 0x007fffe5, 0x003fffd9, 0x007fffe6, 0x007fffe7, 0x00ffffef,
	0x003fffda, 0x001fffdd, 0x000fffe9, 0x003fffdb, 0x003fffdc, 0x007fffe8, 0x007fffe9, 0x001fffde,
	0x007fffea, 0x003fffdd, 0x003fffde, 0x00fffff0, 0x001fffdf, 0x003fffdf, 0x007fffeb, 0x007fffec,
	0x001fffe0, 0x001fffe1, 0x003fffe0, 0x001fffe2, 0x007fffed, 0x003fffe1, 0x007fffee, 0x007fffef,
	0x000fffea, 0x003fffe2, 0x003fffe3, 0x003fffe4, 0x007ffff0, 0x003fffe5, 0x003fffe6, 0x007ffff1,
	0x03ffffe0, 0x03ffffe1, 0x000fffeb, 0x0007fff1, 0x003fffe7, 0x007ffff2, 0x003fffe8, 0x01ffffec,
	0x03ffffe2, 0x03ffffe3, 0x03ffffe4, 0x07ffffde, 0x07ffffdf, 0x03ffffe5, 0x00fffff1, 0x01ffffed,
	0x0007fff2, 0x001fffe3, 0x03ffffe6, 0x07ffffe0, 0x07ffffe1, 0x03ffffe7, 0x07ffffe2, 0x00fffff2,
	0x001fffe4, 0x001fffe5, 0x03ffffe8, 0x03ffffe9, 0x0ffffffd, 0x07ffffe3, 0x07ffffe4, 0x07ffffe5,
	0x000fffec, 0x00fffff3, 0x000fffed, 0x001fffe6, 0x003fffe9, 0x001fffe7, 0x001fffe8, 0x007ffff3,
	0x003fffea, 0x003fffeb, 0x01ffffee, 0x01ffffef, 0x00fffff4, 0x00fffff5, 0x03ffffea, 0x007ffff4,
	0x03ffffeb, 0x07ffffe6, 0x03ffffec, 0x03ffffed, 0x07ffffe7, 0x07ffffe8, 0x07ffffe9, 0x07ffffea,
	0x07ffffeb, 0x0ffffffe, 0x07ffffec, 0x07ffffed, 0x07ffffee, 0x07ffffef, 0x07fffff0, 0x03ffffee,
}
var httpHuffmanSizes = [256]uint8{ // 256B, for huffman encoding
	0x0d, 0x17, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x18, 0x1e, 0x1c, 0x1c, 0x1e, 0x1c, 0x1c,
	0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1e, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c,
	0x06, 0x0a, 0x0a, 0x0c, 0x0d, 0x06, 0x08, 0x0b, 0x0a, 0x0a, 0x08, 0x0b, 0x08, 0x06, 0x06, 0x06,
	0x05, 0x05, 0x05, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x07, 0x08, 0x0f, 0x06, 0x0c, 0x0a,
	0x0d, 0x06, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
	0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x08, 0x07, 0x08, 0x0d, 0x13, 0x0d, 0x0e, 0x06,
	0x0f, 0x05, 0x06, 0x05, 0x06, 0x05, 0x06, 0x06, 0x06, 0x05, 0x07, 0x07, 0x06, 0x06, 0x06, 0x05,
	0x06, 0x07, 0x06, 0x05, 0x05, 0x06, 0x07, 0x07, 0x07, 0x07, 0x07, 0x0f, 0x0b, 0x0e, 0x0d, 0x1c,
	0x14, 0x16, 0x14, 0x14, 0x16, 0x16, 0x16, 0x17, 0x16, 0x17, 0x17, 0x17, 0x17, 0x17, 0x18, 0x17,
	0x18, 0x18, 0x16, 0x17, 0x18, 0x17, 0x17, 0x17, 0x17, 0x15, 0x16, 0x17, 0x16, 0x17, 0x17, 0x18,
	0x16, 0x15, 0x14, 0x16, 0x16, 0x17, 0x17, 0x15, 0x17, 0x16, 0x16, 0x18, 0x15, 0x16, 0x17, 0x17,
	0x15, 0x15, 0x16, 0x15, 0x17, 0x16, 0x17, 0x17, 0x14, 0x16, 0x16, 0x16, 0x17, 0x16, 0x16, 0x17,
	0x1a, 0x1a, 0x14, 0x13, 0x16, 0x17, 0x16, 0x19, 0x1a, 0x1a, 0x1a, 0x1b, 0x1b, 0x1a, 0x18, 0x19,
	0x13, 0x15, 0x1a, 0x1b, 0x1b, 0x1a, 0x1b, 0x18, 0x15, 0x15, 0x1a, 0x1a, 0x1c, 0x1b, 0x1b, 0x1b,
	0x14, 0x18, 0x14, 0x15, 0x16, 0x15, 0x15, 0x17, 0x16, 0x16, 0x19, 0x19, 0x18, 0x18, 0x1a, 0x17,
	0x1a, 0x1b, 0x1a, 0x1a, 0x1b, 0x1b, 0x1b, 0x1b, 0x1b, 0x1c, 0x1b, 0x1b, 0x1b, 0x1b, 0x1b, 0x1a,
}
var httpHuffmanTable = [256][16]struct{ next, sym, emit, end byte }{ // 16K, for huffman decoding
	// TODO
	0x00: {
		{0x04, 0x00, 0, 0}, {0x05, 0x00, 0, 0}, {0x07, 0x00, 0, 0}, {0x08, 0x00, 0, 0},
		{0x0b, 0x00, 0, 0}, {0x0c, 0x00, 0, 0}, {0x10, 0x00, 0, 0}, {0x13, 0x00, 0, 0},
		{0x19, 0x00, 0, 0}, {0x1c, 0x00, 0, 0}, {0x20, 0x00, 0, 0}, {0x23, 0x00, 0, 0},
		{0x2a, 0x00, 0, 0}, {0x31, 0x00, 0, 0}, {0x39, 0x00, 0, 0}, {0x40, 0x00, 0, 1},
	},
	0x01: {
		{0x04, 0x00, 0, 0}, {0x05, 0x00, 0, 0}, {0x07, 0x00, 0, 0}, {0x08, 0x00, 0, 0},
		{0x0b, 0x00, 0, 0}, {0x0c, 0x00, 0, 0}, {0x10, 0x00, 0, 0}, {0x13, 0x00, 0, 0},
		{0x19, 0x00, 0, 0}, {0x1c, 0x00, 0, 0}, {0x20, 0x00, 0, 0}, {0x23, 0x00, 0, 0},
		{0x2a, 0x00, 0, 0}, {0x31, 0x00, 0, 0}, {0x39, 0x00, 0, 0}, {0x40, 0x00, 0, 1},
	},
	0x02: {
		{0x04, 0x00, 0, 0}, {0x05, 0x00, 0, 0}, {0x07, 0x00, 0, 0}, {0x08, 0x00, 0, 0},
		{0x0b, 0x00, 0, 0}, {0x0c, 0x00, 0, 0}, {0x10, 0x00, 0, 0}, {0x13, 0x00, 0, 0},
		{0x19, 0x00, 0, 0}, {0x1c, 0x00, 0, 0}, {0x20, 0x00, 0, 0}, {0x23, 0x00, 0, 0},
		{0x2a, 0x00, 0, 0}, {0x31, 0x00, 0, 0}, {0x39, 0x00, 0, 0}, {0x40, 0x00, 0, 1},
	},
	0x03: {
		{0x04, 0x00, 0, 0}, {0x05, 0x00, 0, 0}, {0x07, 0x00, 0, 0}, {0x08, 0x00, 0, 0},
		{0x0b, 0x00, 0, 0}, {0x0c, 0x00, 0, 0}, {0x10, 0x00, 0, 0}, {0x13, 0x00, 0, 0},
		{0x19, 0x00, 0, 0}, {0x1c, 0x00, 0, 0}, {0x20, 0x00, 0, 0}, {0x23, 0x00, 0, 0},
		{0x2a, 0x00, 0, 0}, {0x31, 0x00, 0, 0}, {0x39, 0x00, 0, 0}, {0x40, 0x00, 0, 1},
	},
	// ...TODO
	0xff: {
		{0x03, 0x16, 1, 0}, {0x06, 0x16, 1, 0}, {0x0a, 0x16, 1, 0}, {0x0f, 0x16, 1, 0},
		{0x18, 0x16, 1, 0}, {0x1f, 0x16, 1, 0}, {0x29, 0x16, 1, 0}, {0x38, 0x16, 1, 1},
		{0xff, 0x00, 0, 0}, {0xff, 0x00, 0, 0}, {0xff, 0x00, 0, 0}, {0xff, 0x00, 0, 0},
		{0xff, 0x00, 0, 0}, {0xff, 0x00, 0, 0}, {0xff, 0x00, 0, 0}, {0xff, 0x00, 0, 0},
	},
}

// httpTableEntry is a dynamic table entry.
type httpTableEntry struct { // 8 bytes
	nameFrom  uint16
	nameEdge  uint16 // nameEdge - nameFrom <= 255
	valueEdge uint16
	totalSize uint16 // nameSize + valueSize + 32
}

// holder is an httpServer or httpClient which holds http connections and streams.
type holder interface {
	Stage() *Stage
	TLSMode() bool
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
}

// stream is the HTTP request-response exchange and the interface for *http[1-3]Stream and *H[1-3]Stream.
type stream interface {
	getHolder() holder

	peerAddr() net.Addr

	unsafeMake(size int) []byte
	smallStack() []byte // a 64 bytes buffer
	makeTempName(p []byte, seconds int64) (from int, edge int)

	setReadDeadline(deadline time.Time) error
	setWriteDeadline(deadline time.Time) error

	read(p []byte) (int, error)
	write(p []byte) (int, error)
	readFull(p []byte) (int, error)
	writev(vector *net.Buffers) (int64, error)

	isBroken() bool
	markBroken()
}

// stream is the trait for httpStream_ and hStream_.
type stream_ struct {
	// Stream states (buffers)
	stockStack [64]byte // a (fake) stack buffer to workaround Go's conservative escape analysis. WARNING: used this as a temp stack scoped memory!
	// Stream states (controlled)
	// Stream states (non-zeros)
	arena region // a region-based memory pool
	// Stream states (zeros)
	httpMode int8 // http mode of current stream. see httpModeXXX
}

func (s *stream_) onUse() { // for non-zeros
	s.arena.init()
	s.httpMode = httpModeNormal
}
func (s *stream_) onEnd() { // for zeros
	s.arena.free()
}

func (s *stream_) unsafeMake(size int) []byte { return s.arena.alloc(size) }
func (s *stream_) smallStack() []byte         { return s.stockStack[:] }

// httpInMessage is a Request or response.
type httpInMessage interface {
	arrayCopy(p []byte) bool
	useHeader(header *pair) bool
	readContent() (p []byte, err error)
	useTrailer(trailer *pair) bool
	walkTrailers(fn func(name []byte, value []byte) bool, withConnection bool) bool
	getSaveContentFilesDir() string
}

// httpInMessage_ is a trait for httpRequest_ and hResponse_.
type httpInMessage_ struct {
	// Assocs
	stream stream        // the stream to which the message belongs
	shell  httpInMessage // *http[1-3]Request or *H[1-3]Response
	// Stream states (buffers)
	stockInput  [_2K]byte // for r.input
	stockArray  [_1K]byte // for r.array
	stockPrimes [76]pair  // for r.primes
	stockExtras [2]pair   // for r.extras
	// Stream states (controlled)
	field          pair     // to overcome the limitation of Go's escape analysis when receiving fields
	contentCodings [4]uint8 // content-encoding flags, controlled by r.nContentCodings. see httpCodingXXX. values: none compress deflate gzip br
	// Stream states (non-zeros)
	input       []byte // bytes of incoming messages (for HTTP/1) or message heads (for HTTP/2 & HTTP/3). [<r.stockInput>/4K/16K]
	array       []byte // store path, queries, extra queries & headers & cookies & trailers, posts, metadata of uploads, and trailers. [<r.stockArray>/4K/16K/64K1/(make <= 1G)]
	primes      []pair // hold prime r.queries->r.array, r.headers->r.input, r.cookies->r.input, r.posts->r.array, and r.trailers->r.array. [<r.stockPrimes>/255]
	extras      []pair // hold extra queries, headers, cookies, and trailers. refers to r.array. [<r.stockExtras>/255]
	contentSize int64  // info of content. >=0: content-length, -1: no content-length header, -2: chunked
	keepAlive   int8   // HTTP/1 only. -1: no connection header, 0: connection close, 1: connection keep-alive
	asResponse  bool   // use message as response?
	headResult  int16  // result of receiving message head. values are same as http status
	// Stream states (zeros)
	headReason      string   // the reason of head result
	inputNext       int32    // HTTP/1 only. next message begins from r.input[r.inputNext]. exists because HTTP/1 messages can be pipelined
	inputEdge       int32    // edge position of current message (for HTTP/1) or head (for HTTP/2 & HTTP/3) is at r.input[r.inputEdge]
	bodyBuffer      []byte   // a window used for receiving content. sizes must be same with r.input for HTTP/1. [HTTP/1=<none>/16K, HTTP/2/3=<none>/4K/16K/64K1]
	contentBlob     []byte   // if loadable, the received and loaded content of current message is at r.contentBlob[:r.sizeReceived]. [<none>/r.input/4K/16K/64K1/(make)]
	contentHeld     *os.File // used by holdContent(), if content is TempFile
	httpInMessage0_          // all values must be zero by default in this struct!
}
type httpInMessage0_ struct { // for fast reset, entirely
	receiveTime     int64 // the time when receiving message (seconds since unix epoch)
	contentTime     int64 // unix timestamp in seconds when first content read operation is performed on this stream
	pBack           int32 // element begins from. for parsing control & headers & content & trailers elements
	pFore           int32 // element spanning to. for parsing control & headers & content & trailers elements
	head            text  // head (control + headers) of current message -> r.input. set after head is received. only for debugging
	imme            text  // HTTP/1 only. immediate data after current message head is at r.input[r.imme.from:r.imme.edge]
	headers         zone  // raw headers ->r.input
	options         zone  // connection options ->r.input
	receiving       int8  // currently receiving. see httpSectionXXX
	versionCode     uint8 // Version1_0, Version1_1, Version2, Version3
	nContentCodings int8  // num of content-encoding flags, controls r.contentCodings
	arrayKind       int8  // kind of current r.array. see arrayKindXXX
	arrayEdge       int32 // next usable position of r.array is at r.array[r.arrayEdge]. used when writing r.array
	iContentLength  uint8 // content-length header ->r.input
	iContentType    uint8 // content-type header ->r.input
	upgradeSocket   bool  // upgrade: websocket?
	contentReceived bool  // is content received?
	contentBlobKind int8  // kind of current r.contentBlob. see httpContentBlobXXX
	maxRecvSeconds  int64 // max seconds to recv message content
	maxContentSize  int64 // max content size allowed for current message. if content is chunked, size is calculated when receiving chunks
	sizeReceived    int64 // bytes of currently received content. for both identity & chunked content receiver
	chunkSize       int64 // left size of current chunk if the chunk is too large to receive in one call. HTTP/1.1 chunked only
	cBack           int32 // for parsing chunked elements. HTTP/1.1 chunked only
	cFore           int32 // for parsing chunked elements. HTTP/1.1 chunked only
	chunkEdge       int32 // edge position of the filled chunked data in r.bodyBuffer. HTTP/1.1 chunked only
	transferChunked bool  // transfer-encoding: chunked? HTTP/1.1 only
	overChunked     bool  // for HTTP/1.1 requests, if chunked content receiver over received in r.bodyBuffer, then r.bodyBuffer will be used as r.input on ends
	trailers        zone  // raw trailers -> r.array. set after trailer section is received and parsed
}

func (r *httpInMessage_) onUse(asResponse bool) { // for non-zeros
	if r.versionCode >= Version2 || asResponse {
		r.input = r.stockInput[:]
	} else {
		// HTTP/1 requests support pipelining, so r.input and r.inputEdge are not reset here.
	}
	r.array = r.stockArray[:]
	r.primes = r.stockPrimes[0:1:cap(r.stockPrimes)] // use append(). r.primes[0] is skipped due to zero value of indexes.
	r.extras = r.stockExtras[0:0:cap(r.stockExtras)] // use append()
	r.contentSize = -1                               // no content-length header
	r.keepAlive = -1                                 // no connection header
	r.asResponse = asResponse
	r.headResult = StatusOK
}
func (r *httpInMessage_) onEnd() { // for zeros
	if r.versionCode >= Version2 || r.asResponse {
		if cap(r.input) != cap(r.stockInput) {
			PutNK(r.input)
		}
		r.input, r.inputEdge = nil, 0
	} else {
		// HTTP/1 requests support pipelining, so r.input and r.inputEdge are not reset here.
	}
	if cap(r.primes) != cap(r.stockPrimes) {
		put255Pairs(r.primes)
		r.primes = nil
	}
	if cap(r.extras) != cap(r.stockExtras) {
		put255Pairs(r.extras)
		r.extras = nil
	}
	if r.arrayKind == arrayKindPool {
		PutNK(r.array)
	}
	r.array = nil

	r.headReason = ""
	if r.inputNext != 0 { // only happens in HTTP/1.1 request pipelining
		if r.overChunked { // only happens in HTTP/1.1 chunked content
			// Use bytes over received in r.bodyBuffer as new r.input.
			// This means the size list for r.bodyBuffer must sync with r.input!
			if cap(r.input) != cap(r.stockInput) {
				PutNK(r.input)
			}
			r.input = r.bodyBuffer // use r.bodyBuffer as new r.input
		}
		// slide r.input
		copy(r.input, r.input[r.inputNext:r.inputEdge]) // r.inputNext and r.inputEdge have already been set
		r.inputEdge -= r.inputNext
		r.inputNext = 0
	} else if r.bodyBuffer != nil { // r.bodyBuffer was used to receive content and failed to free. we free it here.
		PutNK(r.bodyBuffer)
	}
	r.bodyBuffer = nil

	if r.contentBlobKind == httpContentBlobPool {
		PutNK(r.contentBlob)
	}
	r.contentBlob = nil
	if r.contentHeld != nil {
		if Debug() >= 2 {
			fmt.Println("contentFile is closed!!")
		}
		r.contentHeld.Close()
		os.Remove(r.contentHeld.Name())
		r.contentHeld = nil
	}

	r.httpInMessage0_ = httpInMessage0_{}
}

func (r *httpInMessage_) VersionCode() uint8    { return r.versionCode }
func (r *httpInMessage_) Version() string       { return httpVersionStrings[r.versionCode] }
func (r *httpInMessage_) UnsafeVersion() []byte { return httpVersionByteses[r.versionCode] }

func (r *httpInMessage_) addPrime(prime *pair) (edge uint8, ok bool) {
	if len(r.primes) == cap(r.primes) {
		if cap(r.primes) == cap(r.stockPrimes) { // full
			r.primes = get255Pairs()
			r.primes = append(r.primes, r.stockPrimes[:]...)
		} else { // overflow
			return 0, false
		}
	}
	r.primes = append(r.primes, *prime)
	return uint8(len(r.primes)), true
}
func (r *httpInMessage_) addExtra(name string, value string, extraKind uint8) bool {
	nameSize := int32(len(name))
	if nameSize <= 0 || nameSize > 255 { // name size is limited at 255
		return false
	}
	totalSize := nameSize + int32(len(value))
	if totalSize < 0 {
		return false
	}
	if !r._growArray(totalSize) {
		return false
	}
	var extra pair
	extra.setKind(extraKind)
	extra.hash = stringHash(name)
	extra.nameSize = uint8(nameSize)
	extra.nameFrom = r.arrayEdge
	r.arrayEdge += int32(copy(r.array[r.arrayEdge:], name))
	extra.value.from = r.arrayEdge
	r.arrayEdge += int32(copy(r.array[r.arrayEdge:], value))
	extra.value.edge = r.arrayEdge
	if len(r.extras) == cap(r.extras) {
		if cap(r.extras) == cap(r.stockExtras) { // full
			r.extras = get255Pairs()
			r.extras = append(r.extras, r.stockExtras[:]...)
		} else { // overflow
			return false
		}
	}
	r.extras = append(r.extras, extra)
	return true
}

func (r *httpInMessage_) checkConnection(from uint8, edge uint8) bool {
	if r.versionCode >= Version2 {
		r.headResult, r.headReason = StatusBadRequest, "connection header is not allowed in HTTP/2 and HTTP/3"
		return false
	}
	if r.options.isEmpty() {
		r.options.from = from
	}
	r.options.edge = edge
	// RFC 7230 (section 6.1. Connection):
	// Connection        = 1#connection-option
	// connection-option = token
	for i := from; i < edge; i++ {
		vText := r.primes[i].value
		value := r.input[vText.from:vText.edge]
		bytesToLower(value) // connection options are case-insensitive.
		if bytes.Equal(value, httpBytesClose) {
			// Furthermore, the header field-name "Close" has been registered as
			// "reserved", since using that name as an HTTP header field might
			// conflict with the "close" connection option of the Connection header
			// field (Section 6.1).
			r.keepAlive = 0
		} else if bytes.Equal(value, httpBytesKeepAlive) {
			r.keepAlive = 1 // to be compatible with HTTP/1.0
		}
	}
	return true
}
func (r *httpInMessage_) checkTransferEncoding(from uint8, edge uint8) bool {
	if r.versionCode != Version1_1 {
		r.headResult, r.headReason = StatusBadRequest, "transfer-encoding is only allowed in http/1.1"
		return false
	}
	// Transfer-Encoding = 1#transfer-coding
	// transfer-coding   = "chunked" / "compress" / "deflate" / "gzip"
	for i := from; i < edge; i++ {
		vText := r.primes[i].value
		value := r.input[vText.from:vText.edge]
		bytesToLower(value)
		if bytes.Equal(value, httpBytesChunked) {
			r.transferChunked = true
		} else {
			// RFC 7230 (section 3.3.1):
			// A server that receives a request message with a transfer coding it
			// does not understand SHOULD respond with 501 (Not Implemented).
			r.headResult, r.headReason = StatusNotImplemented, "unknown transfer coding"
			return false
		}
	}
	return true
}
func (r *httpInMessage_) checkContentEncoding(from uint8, edge uint8) bool {
	// Content-Encoding = 1#content-coding
	// content-coding   = token
	for i := from; i < edge; i++ {
		if r.nContentCodings == int8(cap(r.contentCodings)) {
			r.headResult, r.headReason = StatusBadRequest, "too many codings in content-encoding"
			return false
		}
		vText := r.primes[i].value
		value := r.input[vText.from:vText.edge]
		bytesToLower(value)
		var coding uint8
		if bytes.Equal(value, httpBytesGzip) {
			coding = httpCodingGzip
		} else if bytes.Equal(value, httpBytesBrotli) {
			coding = httpCodingBrotli
		} else if bytes.Equal(value, httpBytesDeflate) {
			coding = httpCodingDeflate
		} else if bytes.Equal(value, httpBytesCompress) {
			coding = httpCodingCompress
		} else {
			// RFC 7231 (section 3.1.2.2):
			// An origin server MAY respond with a status code of 415 (Unsupported
			// Media Type) if a representation in the request message has a content
			// coding that is not acceptable.

			// TODO: we can be proxies too...
			r.headResult, r.headReason = StatusUnsupportedMediaType, "currently only gzip, deflate, compress, and br are supported"
			return false
		}
		r.contentCodings[r.nContentCodings] = coding
		r.nContentCodings++
	}
	return true
}

func (r *httpInMessage_) checkContentLength(header *pair, index uint8) bool {
	// Content-Length = 1*DIGIT
	// RFC 7230 (section 3.3.2):
	// If a message is received that has multiple Content-Length header
	// fields with field-values consisting of the same decimal value, or a
	// single Content-Length header field with a field value containing a
	// list of identical decimal values (e.g., "Content-Length: 42, 42"),
	// indicating that duplicate Content-Length header fields have been
	// generated or combined by an upstream message processor, then the
	// recipient MUST either reject the message as invalid or replace the
	// duplicated field-values with a single valid Content-Length field
	// containing that decimal value prior to determining the message body
	// length or forwarding the message.
	if r.contentSize == -1 { // r.contentSize can only be -1 or >= 0 here.
		if size, ok := decToI64(r.input[header.value.from:header.value.edge]); ok {
			r.contentSize = size
			r.iContentLength = index
			return true
		}
	}
	// RFC 7230 (section 3.3.3):
	// If a message is received without Transfer-Encoding and with
	// either multiple Content-Length header fields having differing
	// field-values or a single Content-Length header field having an
	// invalid value, then the message framing is invalid and the
	// recipient MUST treat it as an unrecoverable error.  If this is a
	// request message, the server MUST respond with a 400 (Bad Request)
	// status code and then close the connection.
	r.headResult, r.headReason = StatusBadRequest, "bad content-length"
	return false
}
func (r *httpInMessage_) checkContentType(header *pair, index uint8) bool {
	// Content-Type = media-type
	// media-type = type "/" subtype *( OWS ";" OWS parameter )
	// type = token
	// subtype = token
	// parameter = token "=" ( token / quoted-string )
	if r.iContentType == 0 && header.value.notEmpty() {
		r.iContentType = index
		return true
	}
	r.headResult, r.headReason = StatusBadRequest, "bad content-type"
	return false
}
func (r *httpInMessage_) _checkHTTPDate(header *pair, index uint8, pIndex *uint8, toTime *int64) bool {
	if *pIndex == 0 {
		if httpDate, ok := clockParseHTTPDate(r.input[header.value.from:header.value.edge]); ok {
			*pIndex = index
			*toTime = httpDate
			return true
		}
	}
	r.headResult, r.headReason = StatusBadRequest, "bad http-date"
	return false
}

func (r *httpInMessage_) ContentSize() int64 {
	return r.contentSize
}
func (r *httpInMessage_) ContentType() string {
	return string(r.UnsafeContentType())
}
func (r *httpInMessage_) UnsafeContentType() []byte {
	if r.iContentType == 0 {
		return nil
	}
	vType := r.primes[r.iContentType].value
	return r.input[vType.from:vType.edge]
}

func (r *httpInMessage_) H(name string) string {
	value, _ := r.Header(name)
	return value
}
func (r *httpInMessage_) Header(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.headers, extraKindHeader)
	return string(v), ok
}
func (r *httpInMessage_) UnsafeHeader(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.headers, extraKindHeader)
}
func (r *httpInMessage_) HeaderList(name string) (list []string, ok bool) {
	return r.getPairList(name, 0, r.headers, extraKindHeader)
}
func (r *httpInMessage_) Headers() (headers [][2]string) {
	return r.getPairs(r.headers, extraKindHeader)
}
func (r *httpInMessage_) HasHeader(name string) bool {
	_, ok := r.getPair(name, 0, r.headers, extraKindHeader)
	return ok
}
func (r *httpInMessage_) AddHeader(name string, value string) bool {
	// TODO: forbid some important headers, like: if-match, if-none-match, so we don't need to handle them as extra.
	return r.addExtra(name, value, extraKindHeader)
}
func (r *httpInMessage_) DelHeader(name string) (deleted bool) {
	// TODO: forbid some important headers
	return r.delPair(name, 0, r.headers, extraKindHeader)
}
func (r *httpInMessage_) addMultipleHeader(header *pair, must bool) bool {
	// RFC 7230 (section 7):
	// In other words, a recipient MUST accept lists that satisfy the following syntax:
	// #element => [ ( "," / element ) *( OWS "," [ OWS element ] ) ]
	// 1#element => *( "," OWS ) element *( OWS "," [ OWS element ] )
	subHeader := *header
	added := uint8(0)
	value := header.value
	needComma := false
	for {
		haveComma := false
		for value.from < header.value.edge {
			if b := r.input[value.from]; b == ',' {
				haveComma = true
				value.from++
			} else if b == ' ' || b == '\t' {
				value.from++
			} else {
				break
			}
		}
		if value.from == header.value.edge {
			break
		}
		if needComma && !haveComma {
			r.headResult, r.headReason = StatusBadRequest, "comma needed in multi-value header"
			return false
		}
		value.edge = value.from
		if r.input[value.edge] == '"' {
			value.edge++
			// TODO: use index()?
			for value.edge < header.value.edge && r.input[value.edge] != '"' {
				value.edge++
			}
			if value.edge == header.value.edge {
				subHeader.value = value // value is "...
			} else {
				subHeader.value.set(value.from+1, value.edge) // strip ""
				value.edge++
			}
		} else {
			for value.edge < header.value.edge {
				if b := r.input[value.edge]; b == ' ' || b == '\t' || b == ',' {
					break
				} else {
					value.edge++
				}
			}
			subHeader.value = value
		}
		if subHeader.value.notEmpty() {
			if !r.addHeader(&subHeader) {
				return false
			}
			added++
		}
		value.from = value.edge
		needComma = true
	}
	if added == 0 {
		if must {
			r.headResult, r.headReason = StatusBadRequest, "empty element detected in 1#(element)"
			return false
		}
		header.value.zero()
		return r.addHeader(header)
	}
	return true
}
func (r *httpInMessage_) addHeader(header *pair) bool {
	if edge, ok := r.addPrime(header); ok {
		r.headers.edge = edge
		return true
	} else {
		r.headResult, r.headReason = StatusRequestHeaderFieldsTooLarge, "too many headers"
		return false
	}
}
func (r *httpInMessage_) delHeader(name []byte, hash uint16) {
	r.delPair(risky.WeakString(name), hash, r.headers, extraKindHeader)
}
func (r *httpInMessage_) walkHeaders(fn func(name []byte, value []byte) bool, withConnection bool) bool { // used by proxies
	return r._walkFields(r.headers, extraKindHeader, fn, withConnection)
}

func (r *httpInMessage_) SetMaxRecvSeconds(seconds int64) {
	r.maxRecvSeconds = seconds
}

func (r *httpInMessage_) loadContent() { // into memory. [0, r.app.maxMemoryContentSize]
	if r.contentReceived {
		// Content is in r.content already.
		return
	}
	r.contentReceived = true
	switch content := r.recvContent(true).(type) { // retain
	case []byte: // (0, 64K1]. case happens when identity content <= 64K1
		r.contentBlob = content                 // real content is r.contentBlob[:r.sizeReceived]
		r.contentBlobKind = httpContentBlobPool // the returned content is got from pool. put back onEnd
	case TempFile: // [0, r.app.maxMemoryContentSize]. case happens when identity content > 64K1, or content is chunked.
		tempFile := content.(*os.File)
		if r.sizeReceived > 0 { // chunked content may has 0 size, exclude that.
			if r.sizeReceived <= _64K1 {
				r.contentBlob = GetNK(r.sizeReceived) // real content is r.content[:r.sizeReceived]
				r.contentBlobKind = httpContentBlobPool
			} else { // > 64K1, just alloc
				r.contentBlob = make([]byte, r.sizeReceived)
				r.contentBlobKind = httpContentBlobMake
			}
			if _, err := io.ReadFull(tempFile, r.contentBlob[0:r.sizeReceived]); err != nil {
				// TODO: r.app.log
			}
		}
		tempFile.Close()
		os.Remove(tempFile.Name())
	case error:
		// TODO: log error
		r.stream.markBroken()
	}
}
func (r *httpInMessage_) dropContent() {
	switch content := r.recvContent(false).(type) { // don't retain
	case []byte: // (0, 64K1]
		PutNK(content)
	case TempFile: // [0, r.maxContentSize]
		if content != FakeFile { // this must not happen!
			BugExitln("temp file is not fake when dropping content")
		}
	case error:
		// TODO: log error
		r.stream.markBroken()
	}
}
func (r *httpInMessage_) recvContent(retain bool) any { // to []byte (for small content) or TempFile (for large content)
	if r.contentSize > 0 && r.contentSize <= _64K1 { // (0, 64K1]. save to []byte. must be received in a timeout
		timeout := r.stream.getHolder().ReadTimeout()
		if r.maxRecvSeconds > 0 {
			timeout = time.Duration(r.maxRecvSeconds) * time.Second
		}
		if err := r.stream.setReadDeadline(time.Now().Add(timeout)); err != nil {
			return err
		}
		// Since content is small, r.bodyBuffer is not needed, and TempFile is not used either.
		content := GetNK(r.contentSize) // max size of content is 64K1
		r.sizeReceived = int64(r.imme.size())
		if r.sizeReceived > 0 {
			copy(content, r.input[r.imme.from:r.imme.edge])
		}
		for r.sizeReceived < r.contentSize {
			n, err := r.stream.read(content[r.sizeReceived:r.contentSize])
			if err != nil {
				PutNK(content)
				return err
			}
			r.sizeReceived += int64(n)
		}
		return content // []byte, fetched from pool
	}
	// (64K1, r.maxContentSize] when identity, or [0, r.maxContentSize] when chunked. to TempFile
	content, err := r._newTempFile(retain)
	if err != nil {
		return err
	}
	var p []byte
	for {
		p, err = r.shell.readContent()
		if len(p) > 0 { // skip 0, don't write
			if _, e := content.Write(p); e != nil {
				err = e
				goto badRecv
			}
		}
		if err == io.EOF {
			break
		} else if err != nil {
			goto badRecv
		}
	}
	if _, err = content.Seek(0, 0); err != nil {
		goto badRecv
	}
	return content // the TempFile
badRecv:
	content.Close()
	if retain { // the TempFile is not fake, so must remove.
		os.Remove(content.Name())
	}
	return err
}
func (r *httpInMessage_) holdContent() any { // used by proxies
	if r.contentReceived {
		if r.contentHeld == nil { // content is either blob or file
			return r.contentBlob
		} else {
			return r.contentHeld
		}
	}
	r.contentReceived = true
	switch content := r.recvContent(true).(type) { // retain
	case []byte: // (0, 64K1]. case happens when identity content <= 64K1
		r.contentBlob = content
		r.contentBlobKind = httpContentBlobPool // so r.contentBlob can be freed on end
		return r.contentBlob[0:r.sizeReceived]
	case TempFile: // [0, r.app.maxUploadContentSize]. case happens when identity content > 64K1, or content is chunked.
		r.contentHeld = content.(*os.File)
		return r.contentHeld
	case error:
		// TODO: log err
	}
	r.stream.markBroken()
	return nil
}

func (r *httpInMessage_) hasTrailers() bool { // used by proxies
	return r.trailers.notEmpty()
}
func (r *httpInMessage_) T(name string) string {
	value, _ := r.Trailer(name)
	return value
}
func (r *httpInMessage_) Trailer(name string) (value string, ok bool) {
	v, ok := r.getPair(name, 0, r.trailers, extraKindTrailer)
	return string(v), ok
}
func (r *httpInMessage_) UnsafeTrailer(name string) (value []byte, ok bool) {
	return r.getPair(name, 0, r.trailers, extraKindTrailer)
}
func (r *httpInMessage_) TrailerList(name string) (list []string, ok bool) {
	return r.getPairList(name, 0, r.trailers, extraKindTrailer)
}
func (r *httpInMessage_) Trailers() (trailers [][2]string) {
	return r.getPairs(r.trailers, extraKindTrailer)
}
func (r *httpInMessage_) HasTrailer(name string) bool {
	_, ok := r.getPair(name, 0, r.trailers, extraKindTrailer)
	return ok
}
func (r *httpInMessage_) AddTrailer(name string, value string) bool {
	return r.addExtra(name, value, extraKindTrailer)
}
func (r *httpInMessage_) DelTrailer(name string) (deleted bool) {
	return r.delPair(name, 0, r.trailers, extraKindTrailer)
}
func (r *httpInMessage_) addTrailer(trailer *pair) {
	if edge, ok := r.addPrime(trailer); ok {
		r.trailers.edge = edge
	}
	// Ignore too many trailers
}
func (r *httpInMessage_) delTrailer(name []byte, hash uint16) {
	r.delPair(risky.WeakString(name), hash, r.trailers, extraKindTrailer)
}
func (r *httpInMessage_) walkTrailers(fn func(name []byte, value []byte) bool, withConnection bool) bool { // used by proxies
	return r._walkFields(r.trailers, extraKindTrailer, fn, withConnection)
}

func (r *httpInMessage_) UnsafeMake(size int) []byte {
	return r.stream.unsafeMake(size)
}
func (r *httpInMessage_) PeerAddr() net.Addr {
	return r.stream.peerAddr()
}

func (r *httpInMessage_) getPair(name string, hash uint16, primes zone, extraKind uint8) (value []byte, ok bool) {
	if name != "" {
		if hash == 0 {
			hash = stringHash(name)
		}
		for i := primes.from; i < primes.edge; i++ {
			prime := &r.primes[i]
			if prime.hash != hash {
				continue
			}
			p := r._getPlace(prime)
			if prime.nameEqualString(p, name) {
				return p[prime.value.from:prime.value.edge], true
			}
		}
		if extraKind != extraKindNoExtra {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.hash == hash && extra.isKind(extraKind) && extra.nameEqualString(r.array, name) {
					return r.array[extra.value.from:extra.value.edge], true
				}
			}
		}
	}
	return
}
func (r *httpInMessage_) getPairList(name string, hash uint16, primes zone, extraKind uint8) (list []string, ok bool) {
	if name != "" {
		if hash == 0 {
			hash = stringHash(name)
		}
		for i := primes.from; i < primes.edge; i++ {
			prime := &r.primes[i]
			if prime.hash != hash {
				continue
			}
			p := r._getPlace(prime)
			if prime.nameEqualString(p, name) {
				list = append(list, string(p[prime.value.from:prime.value.edge]))
			}
		}
		if extraKind != extraKindNoExtra {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.hash == hash && extra.isKind(extraKind) && extra.nameEqualString(r.array, name) {
					list = append(list, string(r.array[extra.value.from:extra.value.edge]))
				}
			}
		}
		if len(list) > 0 {
			ok = true
		}
	}
	return
}
func (r *httpInMessage_) getPairs(primes zone, extraKind uint8) [][2]string {
	var all [][2]string
	for i := primes.from; i < primes.edge; i++ {
		prime := &r.primes[i]
		p := r._getPlace(prime)
		all = append(all, [2]string{string(p[prime.nameFrom : prime.nameFrom+int32(prime.nameSize)]), string(p[prime.value.from:prime.value.edge])})
	}
	if extraKind != extraKindNoExtra {
		for i := 0; i < len(r.extras); i++ {
			if extra := &r.extras[i]; extra.isKind(extraKind) {
				all = append(all, [2]string{string(r.array[extra.nameFrom : extra.nameFrom+int32(extra.nameSize)]), string(r.array[extra.value.from:extra.value.edge])})
			}
		}
	}
	return all
}
func (r *httpInMessage_) delPair(name string, hash uint16, primes zone, extraKind uint8) (deleted bool) {
	if name != "" {
		if hash == 0 {
			hash = stringHash(name)
		}
		for i := primes.from; i < primes.edge; i++ {
			prime := &r.primes[i]
			if prime.hash != hash {
				continue
			}
			p := r._getPlace(prime)
			if prime.nameEqualString(p, name) {
				prime.zero()
				deleted = true
			}
		}
		if extraKind != extraKindNoExtra {
			for i := 0; i < len(r.extras); i++ {
				if extra := &r.extras[i]; extra.hash == hash && extra.isKind(extraKind) && extra.nameEqualString(r.array, name) {
					extra.zero()
					deleted = true
				}
			}
		}
	}
	return
}
func (r *httpInMessage_) delPrimeAt(i uint8) {
	r.primes[i].zero()
}

func (r *httpInMessage_) arrayPush(b byte) {
	r.array[r.arrayEdge] = b
	if r.arrayEdge++; r.arrayEdge == int32(cap(r.array)) {
		r._growArray(1)
	}
}

func (r *httpInMessage_) _growArray(size int32) bool { // stock->4K->16K->64K1->(128K->...->1G)
	edge := r.arrayEdge + size
	if edge < 0 || edge > _1G { // cannot overflow hard limit: 1G
		return false
	}
	if edge <= int32(cap(r.array)) {
		return true
	}
	arrayKind := r.arrayKind
	var array []byte
	if edge <= _64K1 { // (stock, 64K1]
		r.arrayKind = arrayKindPool
		array = GetNK(int64(edge))
	} else { // > _64K1
		r.arrayKind = arrayKindMake
		if edge <= _128K {
			array = make([]byte, _128K)
		} else if edge <= _256K {
			array = make([]byte, _256K)
		} else if edge <= _512K {
			array = make([]byte, _512K)
		} else if edge <= _1M {
			array = make([]byte, _1M)
		} else if edge <= _2M {
			array = make([]byte, _2M)
		} else if edge <= _4M {
			array = make([]byte, _4M)
		} else if edge <= _8M {
			array = make([]byte, _8M)
		} else if edge <= _16M {
			array = make([]byte, _16M)
		} else if edge <= _32M {
			array = make([]byte, _32M)
		} else if edge <= _64M {
			array = make([]byte, _64M)
		} else if edge <= _128M {
			array = make([]byte, _128M)
		} else if edge <= _256M {
			array = make([]byte, _256M)
		} else if edge <= _512M {
			array = make([]byte, _512M)
		} else { // <= _1G
			array = make([]byte, _1G)
		}
	}
	copy(array, r.array[0:r.arrayEdge])
	if arrayKind == arrayKindPool {
		PutNK(r.array)
	}
	r.array = array
	return true
}
func (r *httpInMessage_) _getPlace(pair *pair) []byte {
	var place []byte
	if pair.inPlace(pairPlaceInput) {
		place = r.input
	} else if pair.inPlace(pairPlaceArray) {
		place = r.array
	} else if pair.inPlace(pairPlaceStatic2) {
		place = http2BytesStatic
	} else if pair.inPlace(pairPlaceStatic3) {
		place = http3BytesStatic
	} else {
		BugExitln("unknown place")
	}
	return place
}

func (r *httpInMessage_) _delHopFields(fields zone, delField func(name []byte, hash uint16)) {
	// These fields should be removed anyway: proxy-connection, keep-alive, te, transfer-encoding, upgrade
	delField(httpBytesProxyConnection, httpHashProxyConnection)
	delField(httpBytesKeepAlive, httpHashKeepAlive)
	if !r.asResponse {
		delField(httpBytesTE, httpHashTE)
	}
	delField(httpBytesTransferEncoding, httpHashTransferEncoding)
	delField(httpBytesUpgrade, httpHashUpgrade)
	for i := r.options.from; i < r.options.edge; i++ {
		prime := &r.primes[i]
		if prime.hash != httpHashConnection || !prime.nameEqualBytes(r.input, httpBytesConnection) {
			continue
		}
		optionName := r.input[prime.value.from:prime.value.edge]
		optionHash := bytesHash(optionName)
		if optionHash == httpHashConnection && bytes.Equal(optionName, httpBytesConnection) { // skip "connection: connection"
			continue
		}
		for j := fields.from; j < fields.edge; j++ {
			field := &r.primes[j]
			if field.hash == optionHash && field.nameEqualBytes(r.input, optionName) {
				field.zero()
			}
		}
		// Note: we don't remove pair ("connection: xxx") itself, since we simply skip it when acting as a proxy.
	}
}
func (r *httpInMessage_) _walkFields(fields zone, extraKind uint8, fn func(name []byte, value []byte) bool, withConnection bool) bool {
	for i := fields.from; i < fields.edge; i++ {
		field := &r.primes[i]
		if field.hash == 0 {
			continue
		}
		p := r._getPlace(field)
		fieldName := p[field.nameFrom : field.nameFrom+int32(field.nameSize)]
		if !withConnection && field.hash == httpHashConnection && bytes.Equal(fieldName, httpBytesConnection) {
			continue
		}
		if !fn(fieldName, p[field.value.from:field.value.edge]) {
			return false
		}
	}
	for i := 0; i < len(r.extras); i++ {
		if extra := &r.extras[i]; extra.isKind(extraKind) {
			fieldName := r.array[extra.nameFrom : extra.nameFrom+int32(extra.nameSize)]
			if !withConnection && extra.hash == httpHashConnection && bytes.Equal(fieldName, httpBytesConnection) {
				continue
			}
			if !fn(fieldName, r.array[extra.value.from:extra.value.edge]) {
				return false
			}
		}
	}
	return true
}

func (r *httpInMessage_) _newTempFile(retain bool) (TempFile, error) {
	if !retain { // since data is not used by upper caller, we don't need to actually write data to file.
		return FakeFile, nil
	}
	filesDir := r.shell.getSaveContentFilesDir() // must ends with '/'
	pathSize := len(filesDir)
	filePath := r.UnsafeMake(pathSize + 19) // 19 bytes is enough for int64
	copy(filePath, filesDir)
	from, edge := r.stream.makeTempName(filePath[pathSize:], r.receiveTime)
	pathSize += copy(filePath[pathSize:], filePath[pathSize+from:pathSize+edge])
	return os.OpenFile(risky.WeakString(filePath[:pathSize]), os.O_RDWR|os.O_CREATE, 0644)
}
func (r *httpInMessage_) _prepareRead(toTime *int64) error {
	now := time.Now()
	if *toTime == 0 {
		*toTime = now.Unix()
	}
	return r.stream.setReadDeadline(now.Add(r.stream.getHolder().ReadTimeout()))
}

const ( // HTTP content blob kinds
	httpContentBlobNone = iota // must be 0
	httpContentBlobInput
	httpContentBlobPool
	httpContentBlobMake
)

var ( // http incoming message errors
	httpReadTooSlow = errors.New("read too slow")
)

// httpOutMessage is a Response or request.
type httpOutMessage interface {
	control() []byte
	header(name []byte) (value []byte, ok bool)
	delHeader(name []byte) (deleted bool)
	addHeader(name []byte, value []byte) bool
	addedHeaders() []byte
	fixedHeaders() []byte
	isForbiddenField(hash uint16, name []byte) bool
	send() error
	doSend(chain Chain) error
	checkPush() error
	pushHeaders() error
	push(chunk *Block) error
	doPush(chain Chain) error
	addTrailer(name []byte, value []byte) bool
	passHeaders() error
	doPass(p []byte) error
	finalizeHeaders()
	finalizeChunked() error
}

// httpOutMessage_ is a trait for httpResponse_ and hRequest_.
type httpOutMessage_ struct {
	// Assocs
	stream stream         // *http[1-3]Stream or *H[1-3]Stream
	shell  httpOutMessage // *http[1-3]Response or *H[1-3]Request
	// Stream states (buffers)
	stockFields [1536]byte // for r.fields
	stockBlock  Block      // for r.content. if content has only one block, this one is used
	// Stream states (controlled)
	edges [128]uint16 // edges of headers or trailers in r.fields. controlled by r.nHeaders or r.nTrailers
	// Stream states (non-zeros)
	fields      []byte // bytes of the headers or trailers. [<r.stockFields>/4K/16K]
	contentSize int64  // -1: not set, -2: chunked encoding, >=0: size
	asRequest   bool   // use message as request?
	content     Chain  // message content, refers to r.stockBlock or a linked list
	// Stream states (zeros)
	vector      net.Buffers // for writev. to overcome the limitation of Go's escape analysis. set when used, reset after stream
	fixedVector [4][]byte   // for sending/pushing message. reset after stream. 96B
	httpOutMessage0_
}
type httpOutMessage0_ struct { // for fast reset, entirely
	maxSendSeconds   int64  // max seconds to send message
	sendTime         int64  // unix timestamp in seconds when first send operation is performed
	fieldsEdge       uint16 // edge of r.fields. max size of r.fields must be <= 16K
	controlEdge      uint16 // edge of control in r.fields. only used by request
	nHeaders         uint8  // num of added headers, <= 255
	nTrailers        uint8  // num of added trailers, <= 255
	forbidContent    bool   // forbid content?
	forbidFraming    bool   // forbid content-length: xxx and transfer-encoding: chunked?
	isSent           bool   // whether the message is sent
	contentTypeAdded bool   // is content-type header added?
}

func (r *httpOutMessage_) onUse(asRequest bool) { // for non-zeros
	r.fields = r.stockFields[:]
	r.contentSize = -1 // not set
	r.asRequest = asRequest
	r.content.PushTail(&r.stockBlock) // r.content has 1 block by default
}
func (r *httpOutMessage_) onEnd() { // for zeros
	if cap(r.fields) != cap(r.stockFields) {
		PutNK(r.fields)
		r.fields = nil
	}
	r.content.free() // also resets r.stockBlock
	r.vector = nil
	r.fixedVector = [4][]byte{}
	r.httpOutMessage0_ = httpOutMessage0_{}
}

func (r *httpOutMessage_) growHeader(size int) (from int, edge int, ok bool) {
	if r.nHeaders == uint8(cap(r.edges)) { // too many headers
		return
	}
	return r._growFields(size)
}
func (r *httpOutMessage_) growTrailer(size int) (from int, edge int, ok bool) {
	if r.nTrailers == uint8(cap(r.edges)) { // too many trailers
		return
	}
	return r._growFields(size)
}
func (r *httpOutMessage_) _growFields(size int) (from int, edge int, ok bool) {
	if size <= 0 || size > _16K { // size allowed: (0, 16K]
		BugExitln("invalid size in _growFields")
	}
	from = int(r.fieldsEdge)
	ceil := r.fieldsEdge + uint16(size)
	tail := ceil + 256 // we reserve 256 bytes at the end of r.fields for finalizeHeaders()
	if ceil < r.fieldsEdge || tail > _16K || tail < ceil {
		// Overflow
		return
	}
	if tail > uint16(cap(r.fields)) { // tail <= _16K
		fields := GetNK(int64(tail)) // 4K/16K
		copy(fields, r.fields[0:r.fieldsEdge])
		if cap(r.fields) != cap(r.stockFields) {
			PutNK(r.fields)
		}
		r.fields = fields
	}
	r.fieldsEdge = ceil
	edge, ok = int(r.fieldsEdge), true
	return
}

func (r *httpOutMessage_) AddContentType(contentType string) bool {
	return r.addContentType(risky.ConstBytes(contentType))
}
func (r *httpOutMessage_) addContentType(contentType []byte) bool {
	if r.contentTypeAdded {
		return true
	}
	if ok := r.shell.addHeader(httpBytesContentType, contentType); ok {
		r.contentTypeAdded = true
		return true
	} else {
		return false
	}
}
func (r *httpOutMessage_) setContentSize(size int64) bool { // only used to set normal content size, not for -1 or -2
	if size >= 0 {
		r.contentSize = size
		return true
	} else {
		return false
	}
}

func (r *httpOutMessage_) Header(name string) (value string, ok bool) {
	v, ok := r.shell.header(risky.ConstBytes(name))
	return string(v), ok
}
func (r *httpOutMessage_) AddHeader(name string, value string) bool {
	return r.AddHeaderBytesByBytes(risky.ConstBytes(name), risky.ConstBytes(value))
}
func (r *httpOutMessage_) AddHeaderBytes(name string, value []byte) bool {
	return r.AddHeaderBytesByBytes(risky.ConstBytes(name), value)
}
func (r *httpOutMessage_) AddHeaderByBytes(name []byte, value string) bool {
	return r.AddHeaderBytesByBytes(name, risky.ConstBytes(value))
}
func (r *httpOutMessage_) AddHeaderBytesByBytes(name []byte, value []byte) bool {
	if hash, ok := bytesCheck(name); ok && !r.shell.isForbiddenField(hash, name) {
		for _, b := range value { // to prevent response splitting
			if b == '\r' || b == '\n' {
				return false
			}
		}
		return r.shell.addHeader(name, value)
	} else {
		return false
	}
}
func (r *httpOutMessage_) DelHeader(name string) bool {
	return r.DelHeaderByBytes(risky.ConstBytes(name))
}
func (r *httpOutMessage_) DelHeaderByBytes(name []byte) (ok bool) {
	if hash, ok := bytesCheck(name); ok && !r.shell.isForbiddenField(hash, name) {
		return r.shell.delHeader(name)
	} else {
		return false
	}
}

func (r *httpOutMessage_) IsSent() bool                    { return r.isSent }
func (r *httpOutMessage_) SetMaxSendSeconds(seconds int64) { r.maxSendSeconds = seconds }

func (r *httpOutMessage_) Send(content string) error {
	return r.SendBytes(risky.ConstBytes(content))
}
func (r *httpOutMessage_) SendBytes(content []byte) error {
	return r.sendBlob(content)
}
func (r *httpOutMessage_) SendFile(contentPath string) error {
	file, err := os.Open(contentPath)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	return r.sendFile(file, info, true)
}
func (r *httpOutMessage_) sendBlob(content []byte) error {
	if err := r.checkSend(); err != nil {
		return err
	}
	r.content.head.SetBlob(content)
	r.contentSize = int64(len(content)) // original size, may be revised
	return r.shell.send()
}
func (r *httpOutMessage_) sendFile(content *os.File, info os.FileInfo, shut bool) error {
	if err := r.checkSend(); err != nil {
		return err
	}
	r.content.head.SetFile(content, info, shut)
	r.contentSize = info.Size() // original size, may be revised
	return r.shell.send()
}
func (r *httpOutMessage_) sendSysf(content system.File, info system.FileInfo, shut bool) error {
	if err := r.checkSend(); err != nil {
		return err
	}
	r.content.head.SetSysf(content, info, shut)
	r.contentSize = info.Size()
	return r.shell.send()
}
func (r *httpOutMessage_) checkSend() error {
	if r.isSent {
		return httpAlreadySent
	}
	r.isSent = true
	return nil
}

func (r *httpOutMessage_) Push(chunk string) error {
	return r.PushBytes(risky.ConstBytes(chunk))
}
func (r *httpOutMessage_) PushBytes(chunk []byte) error {
	return r.pushBlob(chunk)
}
func (r *httpOutMessage_) PushFile(chunkPath string) error {
	file, err := os.Open(chunkPath)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	return r.pushFile(file, info, true)
}
func (r *httpOutMessage_) AddTrailer(name string, value string) bool {
	if !r.isSent { // trailers must be set after headers & content are sent
		return false
	}
	return r.shell.addTrailer(risky.ConstBytes(name), risky.ConstBytes(value))
}
func (r *httpOutMessage_) pushBlob(chunk []byte) error {
	if err := r.shell.checkPush(); err != nil {
		return err
	}
	if len(chunk) == 0 { // empty chunk is not actually sent, since it is used to indicate end of chunks
		return nil
	}
	chunk_ := GetBlock()
	chunk_.SetBlob(chunk)
	return r.shell.push(chunk_)
}
func (r *httpOutMessage_) pushFile(chunk *os.File, info os.FileInfo, shut bool) error {
	if err := r.shell.checkPush(); err != nil {
		return err
	}
	if info.Size() == 0 { // empty chunk is not actually sent, since it is used to indicate end of chunks
		chunk.Close()
		return nil
	}
	chunk_ := GetBlock()
	chunk_.SetFile(chunk, info, shut)
	return r.shell.push(chunk_)
}

func (r *httpOutMessage_) copyHeader(copied *bool, name []byte, value []byte) bool {
	if *copied {
		return true
	}
	if r.shell.addHeader(name, value) {
		*copied = true
		return true
	}
	return false
}
func (r *httpOutMessage_) post(content any, hasTrailers bool) error { // used by proxies
	if contentFile, ok := content.(*os.File); ok {
		fileInfo, err := contentFile.Stat()
		if err != nil {
			contentFile.Close()
			return err
		}
		if hasTrailers { // we must use chunked
			return r.pushFile(contentFile, fileInfo, false)
		} else {
			return r.sendFile(contentFile, fileInfo, false)
		}
	} else if contentBlob, ok := content.([]byte); ok {
		if hasTrailers { // if we supports holding chunked content in buffer, this happens
			return r.pushBlob(contentBlob)
		} else {
			return r.sendBlob(contentBlob)
		}
		return r.sendBlob(contentBlob)
	} else { // nil means no content, but we have to send, so send nil.
		return r.sendBlob(nil)
	}
}

func (r *httpOutMessage_) unsafeMake(size int) []byte {
	return r.stream.unsafeMake(size)
}

func (r *httpOutMessage_) _prepareWrite() error {
	now := time.Now()
	if r.sendTime == 0 {
		r.sendTime = now.Unix()
	}
	return r.stream.setWriteDeadline(now.Add(r.stream.getHolder().WriteTimeout()))
}

var ( // http outgoing message errors
	httpWriteTooSlow       = errors.New("write too slow")
	httpWriteBroken        = errors.New("write broken")
	httpUnknownStatus      = errors.New("unknown status")
	httpStatusForbidden    = errors.New("forbidden status")
	httpAlreadySent        = errors.New("already sent")
	httpContentTooLarge    = errors.New("content too large")
	httpMixIdentityChunked = errors.New("mix identity and chunked")
	httpAddTrailerFailed   = errors.New("add trailer failed")
)

/*

--http[1-3]Stream-->                        ----H[1-3]Stream--->
[CHANGERS]      ^    -------pass/post----->                 ^
                |                                           |
httpInMessage_  | stream                    httpOutMessage_ | stream
^ - stream -----+                           ^ - stream -----+
| - shell ------+                           | - shell ------+
|               | httpInMessage             |               | httpOutMessage
+-httpRequest_  |                           +-hRequest_     |
  ^ - httpConn -+---> http[1-3]Conn           ^ - hConn ----+---> H[1-3]Conn
  |             |                             |             |
  |             v                             |             v
  +-http[1-3]Request                          +-H[1-3]Request
                           \           /
                \           \         /
      1/2/3      \           \       /            1/2/3
   [httpServer]  [Handler]  [httpProxy]        [httpClient]
      1/2/3      /           /       \            1/2/3
                /           /         \
                           /           \
<--http[1-3]Stream--                        <---H[1-3]Stream----
[REVISERS]       ^   <------pass/post------                ^
                 |                                         |
httpOutMessage_  | stream                   httpInMessage_ | stream
^ - stream ------+                          ^ - stream ----+
| - shell -------+                          | - shell -----+
|                | httpOutMessage           |              | httpInMessage
+-httpResponse_  |                          +-hResponse_   |
  ^ - httpConn --+---> http[1-3]Conn          ^ - hConn ---+---> H[1-3]Conn
  |              |                            |            |
  |              v                            |            v
  +-http[1-3]Response                         +-H[1-3]Response

*/
