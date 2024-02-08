// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Exported elements of Hemi.

package hemi

import (
	"github.com/hexinfra/gorox/hemi/internal"

	_ "github.com/hexinfra/gorox/hemi/contrib"
)

// registers

var (
	RegisterAddon = internal.RegisterAddon

	RegisterBackend = internal.RegisterBackend

	RegisterQUICDealet = internal.RegisterQUICDealet
	RegisterTCPSDealet = internal.RegisterTCPSDealet
	RegisterUDPSDealet = internal.RegisterUDPSDealet

	RegisterStater = internal.RegisterStater
	RegisterCacher = internal.RegisterCacher

	RegisterHandlet = internal.RegisterHandlet
	RegisterReviser = internal.RegisterReviser
	RegisterSocklet = internal.RegisterSocklet

	RegisterServer = internal.RegisterServer

	RegisterCronjob = internal.RegisterCronjob
)

var (
	RegisterWebappInit  = internal.RegisterWebappInit
	RegisterServiceInit = internal.RegisterServiceInit
)

// core funcs

var (
	SetDebug = internal.SetDebug
	Debug    = internal.Debug

	SetBaseDir = internal.SetBaseDir
	SetLogsDir = internal.SetLogsDir
	SetTmpsDir = internal.SetTmpsDir
	SetVarsDir = internal.SetVarsDir

	BootFile = internal.BootFile
	BootText = internal.BootText
)

var (
	BaseDir = internal.BaseDir
	LogsDir = internal.LogsDir
	TmpsDir = internal.TmpsDir
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
)

// core types

type (
	Stage = internal.Stage

	Addon = internal.Addon

	Stater  = internal.Stater
	Session = internal.Session

	Server = internal.Server

	Cronjob = internal.Cronjob
)

type (
	Backend = internal.Backend

	QUICBackend = internal.QUICBackend
	QConn       = internal.QConn
	QStream     = internal.QStream

	TCPSBackend = internal.TCPSBackend
	TConn       = internal.TConn

	UDPSBackend = internal.UDPSBackend
	UConn       = internal.UConn

	HRPCBackend = internal.HRPCBackend
	HCall       = internal.HCall
	HReq        = internal.HReq
	HResp       = internal.HResp

	HTTP1Backend = internal.HTTP1Backend
	H1Conn       = internal.H1Conn
	H1Stream     = internal.H1Stream
	H1Request    = internal.H1Request
	H1Response   = internal.H1Response
	H1Socket     = internal.H1Socket

	HTTP2Backend = internal.HTTP2Backend
	H2Conn       = internal.H2Conn
	H2Stream     = internal.H2Stream
	H2Request    = internal.H2Request
	H2Response   = internal.H2Response
	H2Socket     = internal.H2Socket

	HTTP3Backend = internal.HTTP3Backend
	H3Conn       = internal.H3Conn
	H3Stream     = internal.H3Stream
	H3Request    = internal.H3Request
	H3Response   = internal.H3Response
	H3Socket     = internal.H3Socket
)

type (
	QUICRouter = internal.QUICRouter
	QUICDealet = internal.QUICDealet
	QUICConn   = internal.QUICConn
	QUICStream = internal.QUICStream

	TCPSRouter = internal.TCPSRouter
	TCPSDealet = internal.TCPSDealet
	TCPSConn   = internal.TCPSConn

	UDPSRouter = internal.UDPSRouter
	UDPSDealet = internal.UDPSDealet
	UDPSConn   = internal.UDPSConn
)

type (
	Cacher  = internal.Cacher
	Wobject = internal.Wobject

	Webapp   = internal.Webapp
	Handlet  = internal.Handlet
	Handle   = internal.Handle
	Mapper   = internal.Mapper
	Reviser  = internal.Reviser
	Range    = internal.Range
	Piece    = internal.Piece
	Socklet  = internal.Socklet
	Rule     = internal.Rule
	Request  = internal.Request
	Upload   = internal.Upload
	Response = internal.Response
	Cookie   = internal.Cookie
	Socket   = internal.Socket
)

type (
	Service = internal.Service
	Bundlet = internal.Bundlet

	GRPCBridge = internal.GRPCBridge // for implementing gRPC server in exts
)

type ( // core mixins
	Component_ = internal.Component_

	QUICDealet_ = internal.QUICDealet_
	TCPSDealet_ = internal.TCPSDealet_
	UDPSDealet_ = internal.UDPSDealet_

	Stater_ = internal.Stater_
	Cacher_ = internal.Cacher_

	Handlet_ = internal.Handlet_
	Reviser_ = internal.Reviser_
	Socklet_ = internal.Socklet_

	// Server_ = internal.Server_ // Go team says this will be supported in Go 1.23. So wait.
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
