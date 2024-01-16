// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 protocol elements.

package internal

var ( // HTTP/1 slices
	http1BytesContinue             = []byte("HTTP/1.1 100 Continue\r\n\r\n")
	http1BytesConnectionClose      = []byte("connection: close\r\n")
	http1BytesConnectionKeepAlive  = []byte("connection: keep-alive\r\n")
	http1BytesContentTypeStream    = []byte("content-type: application/octet-stream\r\n")
	http1BytesContentTypeHTMLUTF8  = []byte("content-type: text/html; charset=utf-8\r\n")
	http1BytesTransferChunked      = []byte("transfer-encoding: chunked\r\n")
	http1BytesVaryEncoding         = []byte("vary: accept-encoding\r\n")
	http1BytesLocationHTTP         = []byte("location: http://")
	http1BytesLocationHTTPS        = []byte("location: https://")
	http1BytesFixedRequestHeaders  = []byte("client: gorox\r\n\r\n")
	http1BytesFixedResponseHeaders = []byte("server: gorox\r\n\r\n")
	http1BytesZeroCRLF             = []byte("0\r\n")
	http1BytesZeroCRLFCRLF         = []byte("0\r\n\r\n")
)

var http1Template = [16]byte{'H', 'T', 'T', 'P', '/', '1', '.', '1', ' ', 'x', 'x', 'x', ' ', '?', '\r', '\n'}
var http1Controls = [...][]byte{ // size: 512*24B=12K
	// 1XX
	StatusContinue:           []byte("HTTP/1.1 100 Continue\r\n"),
	StatusSwitchingProtocols: []byte("HTTP/1.1 101 Switching Protocols\r\n"),
	StatusProcessing:         []byte("HTTP/1.1 102 Processing\r\n"),
	StatusEarlyHints:         []byte("HTTP/1.1 103 Early Hints\r\n"),
	// 2XX
	StatusOK:                         []byte("HTTP/1.1 200 OK\r\n"),
	StatusCreated:                    []byte("HTTP/1.1 201 Created\r\n"),
	StatusAccepted:                   []byte("HTTP/1.1 202 Accepted\r\n"),
	StatusNonAuthoritativeInfomation: []byte("HTTP/1.1 203 Non-Authoritative Information\r\n"),
	StatusNoContent:                  []byte("HTTP/1.1 204 No Content\r\n"),
	StatusResetContent:               []byte("HTTP/1.1 205 Reset Content\r\n"),
	StatusPartialContent:             []byte("HTTP/1.1 206 Partial Content\r\n"),
	StatusMultiStatus:                []byte("HTTP/1.1 207 Multi-Status\r\n"),
	StatusAlreadyReported:            []byte("HTTP/1.1 208 Already Reported\r\n"),
	StatusIMUsed:                     []byte("HTTP/1.1 226 IM Used\r\n"),
	// 3XX
	StatusMultipleChoices:   []byte("HTTP/1.1 300 Multiple Choices\r\n"),
	StatusMovedPermanently:  []byte("HTTP/1.1 301 Moved Permanently\r\n"),
	StatusFound:             []byte("HTTP/1.1 302 Found\r\n"),
	StatusSeeOther:          []byte("HTTP/1.1 303 See Other\r\n"),
	StatusNotModified:       []byte("HTTP/1.1 304 Not Modified\r\n"),
	StatusUseProxy:          []byte("HTTP/1.1 305 Use Proxy\r\n"),
	StatusTemporaryRedirect: []byte("HTTP/1.1 307 Temporary Redirect\r\n"),
	StatusPermanentRedirect: []byte("HTTP/1.1 308 Permanent Redirect\r\n"),
	// 4XX
	StatusBadRequest:                  []byte("HTTP/1.1 400 Bad Request\r\n"),
	StatusUnauthorized:                []byte("HTTP/1.1 401 Unauthorized\r\n"),
	StatusPaymentRequired:             []byte("HTTP/1.1 402 Payment Required\r\n"),
	StatusForbidden:                   []byte("HTTP/1.1 403 Forbidden\r\n"),
	StatusNotFound:                    []byte("HTTP/1.1 404 Not Found\r\n"),
	StatusMethodNotAllowed:            []byte("HTTP/1.1 405 Method Not Allowed\r\n"),
	StatusNotAcceptable:               []byte("HTTP/1.1 406 Not Acceptable\r\n"),
	StatusProxyAuthenticationRequired: []byte("HTTP/1.1 407 Proxy Authentication Required\r\n"),
	StatusRequestTimeout:              []byte("HTTP/1.1 408 Request Timeout\r\n"),
	StatusConflict:                    []byte("HTTP/1.1 409 Conflict\r\n"),
	StatusGone:                        []byte("HTTP/1.1 410 Gone\r\n"),
	StatusLengthRequired:              []byte("HTTP/1.1 411 Length Required\r\n"),
	StatusPreconditionFailed:          []byte("HTTP/1.1 412 Precondition Failed\r\n"),
	StatusContentTooLarge:             []byte("HTTP/1.1 413 Content Too Large\r\n"),
	StatusURITooLong:                  []byte("HTTP/1.1 414 URI Too Long\r\n"),
	StatusUnsupportedMediaType:        []byte("HTTP/1.1 415 Unsupported Media Type\r\n"),
	StatusRangeNotSatisfiable:         []byte("HTTP/1.1 416 Range Not Satisfiable\r\n"),
	StatusExpectationFailed:           []byte("HTTP/1.1 417 Expectation Failed\r\n"),
	StatusMisdirectedRequest:          []byte("HTTP/1.1 421 Misdirected Request\r\n"),
	StatusUnprocessableEntity:         []byte("HTTP/1.1 422 Unprocessable Entity\r\n"),
	StatusLocked:                      []byte("HTTP/1.1 423 Locked\r\n"),
	StatusFailedDependency:            []byte("HTTP/1.1 424 Failed Dependency\r\n"),
	StatusTooEarly:                    []byte("HTTP/1.1 425 Too Early\r\n"),
	StatusUpgradeRequired:             []byte("HTTP/1.1 426 Upgrade Required\r\n"),
	StatusPreconditionRequired:        []byte("HTTP/1.1 428 Precondition Required\r\n"),
	StatusTooManyRequests:             []byte("HTTP/1.1 429 Too Many Requests\r\n"),
	StatusRequestHeaderFieldsTooLarge: []byte("HTTP/1.1 431 Request Header Fields Too Large\r\n"),
	StatusUnavailableForLegalReasons:  []byte("HTTP/1.1 451 Unavailable For Legal Reasons\r\n"),
	// 5XX
	StatusInternalServerError:           []byte("HTTP/1.1 500 Internal Server Error\r\n"),
	StatusNotImplemented:                []byte("HTTP/1.1 501 Not Implemented\r\n"),
	StatusBadGateway:                    []byte("HTTP/1.1 502 Bad Gateway\r\n"),
	StatusServiceUnavailable:            []byte("HTTP/1.1 503 Service Unavailable\r\n"),
	StatusGatewayTimeout:                []byte("HTTP/1.1 504 Gateway Timeout\r\n"),
	StatusHTTPVersionNotSupported:       []byte("HTTP/1.1 505 HTTP Version Not Supported\r\n"),
	StatusVariantAlsoNegotiates:         []byte("HTTP/1.1 506 Variant Also Negotiates\r\n"),
	StatusInsufficientStorage:           []byte("HTTP/1.1 507 Insufficient Storage\r\n"),
	StatusLoopDetected:                  []byte("HTTP/1.1 508 Loop Detected\r\n"),
	StatusNotExtended:                   []byte("HTTP/1.1 510 Not Extended\r\n"),
	StatusNetworkAuthenticationRequired: []byte("HTTP/1.1 511 Network Authentication Required\r\n"),
}
