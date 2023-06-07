// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Exported elements of Hemi.

package hemi

import (
	"github.com/hexinfra/gorox/hemi/internal"

	_ "github.com/hexinfra/gorox/hemi/contrib"
)

var ( // registers
	RegisterRunner = internal.RegisterRunner

	RegisterBackend = internal.RegisterBackend

	RegisterQUICDealer = internal.RegisterQUICDealer
	RegisterQUICEditor = internal.RegisterQUICEditor
	RegisterTCPSDealer = internal.RegisterTCPSDealer
	RegisterTCPSEditor = internal.RegisterTCPSEditor
	RegisterUDPSDealer = internal.RegisterUDPSDealer
	RegisterUDPSEditor = internal.RegisterUDPSEditor

	RegisterStater = internal.RegisterStater
	RegisterStorer = internal.RegisterStorer

	RegisterAppInit = internal.RegisterAppInit
	RegisterHandlet = internal.RegisterHandlet
	RegisterReviser = internal.RegisterReviser
	RegisterSocklet = internal.RegisterSocklet

	RegisterSvcInit = internal.RegisterSvcInit

	RegisterServer = internal.RegisterServer

	RegisterCronjob = internal.RegisterCronjob
)

var ( // core funcs
	SetDebug = internal.SetDebug
	Debug    = internal.Debug

	SetBaseDir = internal.SetBaseDir
	SetLogsDir = internal.SetLogsDir
	SetTempDir = internal.SetTempDir
	SetVarsDir = internal.SetVarsDir

	BaseDir = internal.BaseDir
	LogsDir = internal.LogsDir
	TempDir = internal.TempDir
	VarsDir = internal.VarsDir

	Print   = internal.Print
	Println = internal.Println
	Printf  = internal.Printf
	Error   = internal.Error
	Errorln = internal.Errorln
	Errorf  = internal.Errorf

	BugExitln = internal.BugExitln
	BugExitf  = internal.BugExitf
	UseExitln = internal.UseExitln
	UseExitf  = internal.UseExitf
	EnvExitln = internal.EnvExitln
	EnvExitf  = internal.EnvExitf

	FromFile = internal.FromFile
	FromText = internal.FromText
)

type ( // core types
	Stage = internal.Stage

	Runner = internal.Runner

	Backend = internal.Backend

	QUICOutgate = internal.QUICOutgate
	QUICBackend = internal.QUICBackend
	QConnection = internal.QConnection
	QStream     = internal.QStream
	QOneway     = internal.QOneway

	QUDSOutgate = internal.QUDSOutgate
	QUDSBackend = internal.QUDSBackend
	XConnection = internal.XConnection
	XStream     = internal.XStream
	XOneway     = internal.XOneway

	TCPSOutgate = internal.TCPSOutgate
	TCPSBackend = internal.TCPSBackend
	TConn       = internal.TConn

	TUDSOutgate = internal.TUDSOutgate
	TUDSBackend = internal.TUDSBackend
	XConn       = internal.XConn

	UDPSOutgate = internal.UDPSOutgate
	UDPSBackend = internal.UDPSBackend
	ULink       = internal.ULink

	UUDSOutgate = internal.UUDSOutgate
	UUDSBackend = internal.UUDSBackend
	XLink       = internal.XLink

	HRPCOutgate = internal.HRPCOutgate
	HRPCBackend = internal.HRPCBackend
	HCall       = internal.HCall
	HReq        = internal.HReq
	HResp       = internal.HResp

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

	HWEBOutgate = internal.HWEBOutgate
	HWEBBackend = internal.HWEBBackend
	HExchan     = internal.HExchan
	HRequest    = internal.HRequest
	HResponse   = internal.HResponse

	QUICMesher     = internal.QUICMesher
	QUICDealer     = internal.QUICDealer
	QUICEditor     = internal.QUICEditor
	QUICConnection = internal.QUICConnection

	TCPSMesher = internal.TCPSMesher
	TCPSDealer = internal.TCPSDealer
	TCPSEditor = internal.TCPSEditor
	TCPSConn   = internal.TCPSConn

	UDPSMesher = internal.UDPSMesher
	UDPSDealer = internal.UDPSDealer
	UDPSEditor = internal.UDPSEditor
	UDPSLink   = internal.UDPSLink

	Stater  = internal.Stater
	Session = internal.Session

	Storer  = internal.Storer
	Hobject = internal.Hobject

	App      = internal.App
	Handlet  = internal.Handlet
	Handle   = internal.Handle
	Router   = internal.Router
	Reviser  = internal.Reviser
	Piece    = internal.Piece
	Socklet  = internal.Socklet
	Rule     = internal.Rule
	Request  = internal.Request
	Upload   = internal.Upload
	Response = internal.Response
	Cookie   = internal.Cookie
	Socket   = internal.Socket

	Svc     = internal.Svc
	Bundlet = internal.Bundlet

	Server = internal.Server

	GRPCBridge   = internal.GRPCBridge   // for implementing gRPC server in exts
	ThriftBridge = internal.ThriftBridge // for implementing Thrift server in exts

	Cronjob = internal.Cronjob
)

type ( // core mixins
	Component_ = internal.Component_

	QUICDealer_ = internal.QUICDealer_
	QUICEditor_ = internal.QUICEditor_

	TCPSDealer_ = internal.TCPSDealer_
	TCPSEditor_ = internal.TCPSEditor_

	UDPSDealer_ = internal.UDPSDealer_
	UDPSEditor_ = internal.UDPSEditor_

	Stater_ = internal.Stater_
	Storer_ = internal.Storer_

	Handlet_ = internal.Handlet_
	Reviser_ = internal.Reviser_
	Socklet_ = internal.Socklet_

	Server_ = internal.Server_
	Gate_   = internal.Gate_

	Cronjob_ = internal.Cronjob_
)

const ( // core constants
	Version = internal.Version

	CodeBug = internal.CodeBug
	CodeUse = internal.CodeUse
	CodeEnv = internal.CodeEnv
)

const ( // protocol constants
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
