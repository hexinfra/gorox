// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Exported elements of internal.

package hemi

import (
	"github.com/hexinfra/gorox/hemi/internal"
	_ "github.com/hexinfra/gorox/hemi/standard" // preloaded components
)

const Version = "0.1.0-dev"

var ( // core funcs
	RegisterRunner     = internal.RegisterRunner
	RegisterQUICDealet = internal.RegisterQUICDealet
	RegisterQUICEditor = internal.RegisterQUICEditor
	RegisterTCPSDealet = internal.RegisterTCPSDealet
	RegisterTCPSEditor = internal.RegisterTCPSEditor
	RegisterUDPSDealet = internal.RegisterUDPSDealet
	RegisterUDPSEditor = internal.RegisterUDPSEditor
	RegisterStater     = internal.RegisterStater
	RegisterCacher     = internal.RegisterCacher
	RegisterAppInit    = internal.RegisterAppInit
	RegisterHandlet    = internal.RegisterHandlet
	RegisterReviser    = internal.RegisterReviser
	RegisterSocklet    = internal.RegisterSocklet
	RegisterSvcInit    = internal.RegisterSvcInit
	RegisterServer     = internal.RegisterServer
	RegisterCronjob    = internal.RegisterCronjob

	IsDebug = internal.IsDebug
	BaseDir = internal.BaseDir
	DataDir = internal.DataDir
	LogsDir = internal.LogsDir
	TempDir = internal.TempDir

	SetDebug   = internal.SetDebug
	SetBaseDir = internal.SetBaseDir
	SetDataDir = internal.SetDataDir
	SetLogsDir = internal.SetLogsDir
	SetTempDir = internal.SetTempDir

	Debug   = internal.Debug
	Debugln = internal.Debugln
	Debugf  = internal.Debugf

	BugExitln = internal.BugExitln
	BugExitf  = internal.BugExitf
	UseExitln = internal.UseExitln
	UseExitf  = internal.UseExitf
	EnvExitln = internal.EnvExitln
	EnvExitf  = internal.EnvExitf

	ApplyFile = internal.ApplyFile
	ApplyText = internal.ApplyText

	NewSimpleRouter = internal.NewSimpleRouter
)

type ( // core types
	Stage = internal.Stage

	Runner = internal.Runner

	HTTP1Outgate = internal.HTTP1Outgate
	HTTP1Backend = internal.HTTP1Backend
	H1Conn       = internal.H1Conn
	H1Stream     = internal.H1Stream
	H1Request    = internal.H1Request
	H1Response   = internal.H1Response
	H1Socket     = internal.H1Socket

	HTTP2Outgate = internal.HTTP2Outgate
	HTTP2Backend = internal.HTTP2Backend
	H2Conn       = internal.H2Conn
	H2Stream     = internal.H2Stream
	H2Request    = internal.H2Request
	H2Response   = internal.H2Response
	H2Socket     = internal.H2Socket

	HTTP3Outgate = internal.HTTP3Outgate
	HTTP3Backend = internal.HTTP3Backend
	H3Conn       = internal.H3Conn
	H3Stream     = internal.H3Stream
	H3Request    = internal.H3Request
	H3Response   = internal.H3Response
	H3Socket     = internal.H3Socket

	QUICOutgate = internal.QUICOutgate
	QUICBackend = internal.QUICBackend
	QConn       = internal.QConn

	TCPSOutgate = internal.TCPSOutgate
	TCPSBackend = internal.TCPSBackend
	TConn       = internal.TConn

	UDPSOutgate = internal.UDPSOutgate
	UDPSBackend = internal.UDPSBackend
	UConn       = internal.UConn

	UnixOutgate = internal.UnixOutgate
	UnixBackend = internal.UnixBackend
	XConn       = internal.XConn

	PBackend = internal.PBackend // TCPSBackend | UnixBackend
	PConn    = internal.PConn    // TConn | XConn

	QUICMesher = internal.QUICMesher
	QUICDealet = internal.QUICDealet
	QUICEditor = internal.QUICEditor
	QUICConn   = internal.QUICConn

	TCPSMesher = internal.TCPSMesher
	TCPSDealet = internal.TCPSDealet
	TCPSEditor = internal.TCPSEditor
	TCPSConn   = internal.TCPSConn

	UDPSMesher = internal.UDPSMesher
	UDPSDealet = internal.UDPSDealet
	UDPSEditor = internal.UDPSEditor
	UDPSConn   = internal.UDPSConn

	Stater  = internal.Stater
	Session = internal.Session

	Cacher  = internal.Cacher
	Hobject = internal.Hobject

	Block = internal.Block

	App       = internal.App
	Handlet   = internal.Handlet
	Handle    = internal.Handle
	Router    = internal.Router
	Reviser   = internal.Reviser
	Socklet   = internal.Socklet
	Rule      = internal.Rule
	Request   = internal.Request
	Upload    = internal.Upload
	Response  = internal.Response
	SetCookie = internal.SetCookie
	Socket    = internal.Socket

	Svc        = internal.Svc        // supports both gRPC and HRPC
	GRPCServer = internal.GRPCServer // for implementing grpc server in exts

	Server  = internal.Server
	Cronjob = internal.Cronjob
)

type ( // core mixins
	Component_ = internal.Component_

	Stater_     = internal.Stater_
	Cacher_     = internal.Cacher_
	QUICDealet_ = internal.QUICDealet_
	QUICEditor_ = internal.QUICEditor_
	TCPSDealet_ = internal.TCPSDealet_
	TCPSEditor_ = internal.TCPSEditor_
	UDPSDealet_ = internal.UDPSDealet_
	UDPSEditor_ = internal.UDPSEditor_
	Server_     = internal.Server_
	Gate_       = internal.Gate_
	Handlet_    = internal.Handlet_
	Reviser_    = internal.Reviser_
	Socklet_    = internal.Socklet_
	Cronjob_    = internal.Cronjob_
)

const ( // core constants
	CodeBug = internal.CodeBug
	CodeUse = internal.CodeUse
	CodeEnv = internal.CodeEnv

	Version1_0 = internal.Version1_0
	Version1_1 = internal.Version1_1
	Version2   = internal.Version2
	Version3   = internal.Version3

	SchemeHTTP  = internal.SchemeHTTP
	SchemeHTTPS = internal.SchemeHTTPS

	// Known methods
	MethodGET     = internal.MethodGET
	MethodHEAD    = internal.MethodHEAD
	MethodPOST    = internal.MethodPOST
	MethodPUT     = internal.MethodPUT
	MethodDELETE  = internal.MethodDELETE
	MethodCONNECT = internal.MethodCONNECT
	MethodOPTIONS = internal.MethodOPTIONS
	MethodTRACE   = internal.MethodTRACE
	MethodPATCH   = internal.MethodPATCH
	MethodLINK    = internal.MethodLINK
	MethodUNLINK  = internal.MethodUNLINK
	MethodQUERY   = internal.MethodQUERY

	// 1XX
	StatusContinue           = internal.StatusContinue
	StatusSwitchingProtocols = internal.StatusSwitchingProtocols
	StatusProcessing         = internal.StatusProcessing
	StatusEarlyHints         = internal.StatusEarlyHints
	// 2XX
	StatusOK                         = internal.StatusOK
	StatusCreated                    = internal.StatusCreated
	StatusAccepted                   = internal.StatusAccepted
	StatusNonAuthoritativeInfomation = internal.StatusNonAuthoritativeInfomation
	StatusNoContent                  = internal.StatusNoContent
	StatusResetContent               = internal.StatusResetContent
	StatusPartialContent             = internal.StatusPartialContent
	StatusMultiStatus                = internal.StatusMultiStatus
	StatusAlreadyReported            = internal.StatusAlreadyReported
	StatusIMUsed                     = internal.StatusIMUsed
	// 3XX
	StatusMultipleChoices   = internal.StatusMultipleChoices
	StatusMovedPermanently  = internal.StatusMovedPermanently
	StatusFound             = internal.StatusFound
	StatusSeeOther          = internal.StatusSeeOther
	StatusNotModified       = internal.StatusNotModified
	StatusUseProxy          = internal.StatusUseProxy
	StatusTemporaryRedirect = internal.StatusTemporaryRedirect
	StatusPermanentRedirect = internal.StatusPermanentRedirect
	// 4XX
	StatusBadRequest                  = internal.StatusBadRequest
	StatusUnauthorized                = internal.StatusUnauthorized
	StatusPaymentRequired             = internal.StatusPaymentRequired
	StatusForbidden                   = internal.StatusForbidden
	StatusNotFound                    = internal.StatusNotFound
	StatusMethodNotAllowed            = internal.StatusMethodNotAllowed
	StatusNotAcceptable               = internal.StatusNotAcceptable
	StatusProxyAuthenticationRequired = internal.StatusProxyAuthenticationRequired
	StatusRequestTimeout              = internal.StatusRequestTimeout
	StatusConflict                    = internal.StatusConflict
	StatusGone                        = internal.StatusGone
	StatusLengthRequired              = internal.StatusLengthRequired
	StatusPreconditionFailed          = internal.StatusPreconditionFailed
	StatusContentTooLarge             = internal.StatusContentTooLarge
	StatusURITooLong                  = internal.StatusURITooLong
	StatusUnsupportedMediaType        = internal.StatusUnsupportedMediaType
	StatusRangeNotSatisfiable         = internal.StatusRangeNotSatisfiable
	StatusExpectationFailed           = internal.StatusExpectationFailed
	StatusMisdirectedRequest          = internal.StatusMisdirectedRequest
	StatusUnprocessableEntity         = internal.StatusUnprocessableEntity
	StatusLocked                      = internal.StatusLocked
	StatusFailedDependency            = internal.StatusFailedDependency
	StatusTooEarly                    = internal.StatusTooEarly
	StatusUpgradeRequired             = internal.StatusUpgradeRequired
	StatusPreconditionRequired        = internal.StatusPreconditionRequired
	StatusTooManyRequests             = internal.StatusTooManyRequests
	StatusRequestHeaderFieldsTooLarge = internal.StatusRequestHeaderFieldsTooLarge
	StatusUnavailableForLegalReasons  = internal.StatusUnavailableForLegalReasons
	// 5XX
	StatusInternalServerError           = internal.StatusInternalServerError
	StatusNotImplemented                = internal.StatusNotImplemented
	StatusBadGateway                    = internal.StatusBadGateway
	StatusServiceUnavailable            = internal.StatusServiceUnavailable
	StatusGatewayTimeout                = internal.StatusGatewayTimeout
	StatusHTTPVersionNotSupported       = internal.StatusHTTPVersionNotSupported
	StatusVariantAlsoNegotiates         = internal.StatusVariantAlsoNegotiates
	StatusInsufficientStorage           = internal.StatusInsufficientStorage
	StatusLoopDetected                  = internal.StatusLoopDetected
	StatusNotExtended                   = internal.StatusNotExtended
	StatusNetworkAuthenticationRequired = internal.StatusNetworkAuthenticationRequired
)
