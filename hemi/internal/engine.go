// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Basic engine elements exist between multiple stages.

package internal

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const Version = "0.2.0-dev"

var ( // global variables shared between stages
	_baseOnce sync.Once    // protects _baseDir
	_baseDir  atomic.Value // directory of the executable
	_logsOnce sync.Once    // protects _logsDir
	_logsDir  atomic.Value // directory of the log files
	_tempOnce sync.Once    // protects _tempDir
	_tempDir  atomic.Value // directory of the temp files
	_varsOnce sync.Once    // protects _varsDir
	_varsDir  atomic.Value // directory of the run-time data
)

func SetBaseDir(dir string) { // only once!
	_baseOnce.Do(func() {
		_baseDir.Store(dir)
	})
}
func SetLogsDir(dir string) { // only once!
	_logsOnce.Do(func() {
		_logsDir.Store(dir)
		_mkdir(dir)
	})
}
func SetTempDir(dir string) { // only once!
	_tempOnce.Do(func() {
		_tempDir.Store(dir)
		_mkdir(dir)
	})
}
func SetVarsDir(dir string) { // only once!
	_varsOnce.Do(func() {
		_varsDir.Store(dir)
		_mkdir(dir)
	})
}
func _mkdir(dir string) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf(err.Error())
		os.Exit(0)
	}
}

func BaseDir() string { return _baseDir.Load().(string) }
func LogsDir() string { return _logsDir.Load().(string) }
func TempDir() string { return _tempDir.Load().(string) }
func VarsDir() string { return _varsDir.Load().(string) }

var _debug atomic.Int32 // debug level

func SetDebug(level int32)     { _debug.Store(level) }
func IsDebug(level int32) bool { return _debug.Load() >= level }

func Debug(args ...any)                 { fmt.Print(args...) }
func Debugln(args ...any)               { fmt.Println(args...) }
func Debugf(format string, args ...any) { fmt.Printf(format, args...) }

const ( // exit codes. keep sync with ../hemi.go
	CodeBug = 20
	CodeUse = 21
	CodeEnv = 22
)

func BugExitln(args ...any) { exitln(CodeBug, "[BUG] ", args...) }
func UseExitln(args ...any) { exitln(CodeUse, "[USE] ", args...) }
func EnvExitln(args ...any) { exitln(CodeEnv, "[ENV] ", args...) }

func BugExitf(format string, args ...any) { exitf(CodeBug, "[BUG] ", format, args...) }
func UseExitf(format string, args ...any) { exitf(CodeUse, "[USE] ", format, args...) }
func EnvExitf(format string, args ...any) { exitf(CodeEnv, "[ENV] ", format, args...) }

func exitln(exitCode int, prefix string, args ...any) {
	fmt.Fprint(os.Stderr, prefix)
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(exitCode)
}
func exitf(exitCode int, prefix, format string, args ...any) {
	fmt.Fprintf(os.Stderr, prefix+format, args...)
	os.Exit(exitCode)
}

func ApplyText(text string) (*Stage, error) {
	_checkDirs()
	c := _newConfig()
	return c.applyText(text)
}
func ApplyFile(base string, file string) (*Stage, error) {
	_checkDirs()
	c := _newConfig()
	return c.applyFile(base, file)
}
func _checkDirs() {
	if _baseDir.Load() == nil || _logsDir.Load() == nil || _tempDir.Load() == nil || _varsDir.Load() == nil {
		UseExitln("baseDir, logsDir, tempDir, and varsDir must all be set")
	}
}
func _newConfig() *config {
	c := new(config)
	c.init(map[string]string{
		"baseDir": BaseDir(),
		"logsDir": LogsDir(),
		"tempDir": TempDir(),
		"varsDir": VarsDir(),
	}, map[string]int16{
		// general conn vars. keep sync with mesher_quic.go, mesher_tcps.go, and mesher_udps.go
		"srcHost": 0,
		"srcPort": 1,

		// general tcps & udps conn vars. keep sync with mesher_tcps.go and mesher_udps.go
		"transport": 2, // tcp/udp, tls/dtls

		// quic conn vars. keep sync with quicConnVariables in mesher_quic.go
		// quic stream vars. keep sync with quicConnVariables in mesher_quic.go

		// tcps conn vars. keep sync with tcpsConnVariables in mesher_tcps.go
		"serverName": 3,
		"nextProto":  4,

		// udps conn vars. keep sync with udpsConnVariables in mesher_udps.go

		// http request vars. keep sync with serverRequestVariables in web_server.go
		"method":      0, // GET, POST, ...
		"scheme":      1, // http, https
		"authority":   2, // example.com, example.org:8080
		"hostname":    3, // example.com, example.org
		"colonPort":   4, // :80, :8080
		"path":        5, // /abc, /def/
		"uri":         6, // /abc?x=y, /%cc%dd?y=z&z=%ff
		"encodedPath": 7, // /abc, /%cc%dd
		"queryString": 8, // ?x=y, ?y=z&z=%ff
		"contentType": 9, // application/json
	}, signedComps)
	return c
}

// config applies configuration and creates a new stage.
type config struct {
	// States
	constants   map[string]string // defined constants
	varCodes    map[string]int16  // defined var codes
	signedComps map[string]int16  // defined signed comps
	tokens      []token           // the token list
	index       int               // token index
	limit       int               // limit of token index
	counter     int               // the name for components without a name
}

func (c *config) init(constants map[string]string, varCodes map[string]int16, signedComps map[string]int16) {
	c.constants = constants
	c.varCodes = varCodes
	c.signedComps = signedComps
}

func (c *config) applyText(text string) (stage *Stage, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
	}()
	var l lexer
	c.tokens = l.scanText(text)
	c.evaluate()
	return c.parse()
}
func (c *config) applyFile(base string, path string) (stage *Stage, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
	}()
	var l lexer
	c.tokens = l.scanFile(base, path)
	c.evaluate()
	return c.parse()
}

func (c *config) show() {
	for i := 0; i < len(c.tokens); i++ {
		token := &c.tokens[i]
		fmt.Printf("kind=%16s info=%2d line=%4d file=%s    %s\n", token.name(), token.info, token.line, token.file, token.text)
	}
}
func (c *config) evaluate() {
	for i := 0; i < len(c.tokens); i++ {
		token := &c.tokens[i]
		switch token.kind {
		case tokenIdentifier: // some identifiers are components
			if comp, ok := c.signedComps[token.text]; ok {
				token.info = comp
			}
		case tokenConstant: // evaluate constants
			if text, ok := c.constants[token.text]; ok {
				token.kind = tokenString
				token.text = text
			}
		case tokenVariable: // evaluate variable codes
			if code, ok := c.varCodes[token.text]; ok {
				token.info = code
			}
		}
	}
}

func (c *config) current() *token           { return &c.tokens[c.index] }
func (c *config) currentIs(kind int16) bool { return c.tokens[c.index].kind == kind }
func (c *config) nextIs(kind int16) bool {
	if c.index == c.limit {
		return false
	}
	return c.tokens[c.index+1].kind == kind
}
func (c *config) expect(kind int16) *token {
	current := &c.tokens[c.index]
	if current.kind != kind {
		panic(fmt.Errorf("config: expect %s, but get %s=%s (in line %d)\n", tokenNames[kind], tokenNames[current.kind], current.text, current.line))
	}
	return current
}
func (c *config) forwardExpect(kind int16) *token {
	if c.index++; c.index == c.limit {
		panic(errors.New("config: unexpected EOF"))
	}
	return c.expect(kind)
}
func (c *config) forward() *token {
	if c.index++; c.index == c.limit {
		panic(errors.New("config: unexpected EOF"))
	}
	return &c.tokens[c.index]
}
func (c *config) newName() string {
	c.counter++
	return strconv.Itoa(c.counter)
}

func (c *config) parse() (stage *Stage, err error) {
	if current := c.current(); current.kind == tokenIdentifier && current.info == compStage {
		stage = createStage()
		stage.setParent(nil)
		c.parseStage(stage)
		return stage, nil
	}
	panic(errors.New("config error: root component is not stage"))
}

func (c *config) parseStage(stage *Stage) { // stage {}
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c.parseAssign(current, stage)
			continue
		}
		if current.kind != tokenIdentifier {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in stage\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compFixture:
			c.parseFixture(current, stage)
		case compUniture:
			c.parseUniture(current, stage)
		case compBackend:
			c.parseBackend(current, stage)
		case compQUICMesher:
			c.parseQUICMesher(stage)
		case compTCPSMesher:
			c.parseTCPSMesher(stage)
		case compUDPSMesher:
			c.parseUDPSMesher(stage)
		case compStater:
			c.parseStater(current, stage)
		case compCacher:
			c.parseCacher(current, stage)
		case compApp:
			c.parseApp(current, stage)
		case compSvc:
			c.parseSvc(current, stage)
		case compServer:
			c.parseServer(current, stage)
		case compCronjob:
			c.parseCronjob(current, stage)
		default:
			panic(fmt.Errorf("unknown component '%s' in stage\n", current.text))
		}
	}
}
func (c *config) parseFixture(sign *token, stage *Stage) { // xxxFixture {}
	fixtureSign := sign.text
	fixture := stage.fixture(fixtureSign)
	if fixture == nil {
		panic(errors.New("config error: unknown fixture: " + fixtureSign))
	}
	fixture.setParent(stage)
	c.forward()
	c.parseLeaf(fixture)
}
func (c *config) parseUniture(sign *token, stage *Stage) { // xxxUniture {}
	uniture := stage.createUniture(sign.text)
	uniture.setParent(stage)
	c.forward()
	c.parseLeaf(uniture)
}
func (c *config) parseBackend(sign *token, stage *Stage) { // xxxBackend <name> {}
	parseComponent0(c, sign, stage, stage.createBackend)
}
func (c *config) parseQUICMesher(stage *Stage) { // quicMesher <name> {}
	mesherName := c.forwardExpect(tokenString)
	mesher := stage.createQUICMesher(mesherName.text)
	mesher.setParent(stage)
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c.parseAssign(current, mesher)
			continue
		}
		if current.kind != tokenIdentifier {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in quicMesher\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compQUICDealer:
			c.parseQUICDealer(current, mesher, nil)
		case compQUICEditor:
			c.parseQUICEditor(current, mesher, nil)
		case compCase:
			c.parseQUICCase(mesher)
		default:
			panic(fmt.Errorf("unknown component '%s' in quicMesher\n", current.text))
		}
	}
}
func (c *config) parseQUICDealer(sign *token, mesher *QUICMesher, kase *quicCase) { // qqqDealer <name> {}, qqqDealer {}
	parseComponent1(c, sign, mesher, mesher.createDealer, kase, kase.addDealer)
}
func (c *config) parseQUICEditor(sign *token, mesher *QUICMesher, kase *quicCase) { // qqqEditor <name> {}, qqqEditor {}
	parseComponent1(c, sign, mesher, mesher.createEditor, kase, kase.addEditor)
}
func (c *config) parseQUICCase(mesher *QUICMesher) { // case <name> {}, case <name> <cond> {}, case <cond> {}, case {}
	kase := mesher.createCase(c.newName()) // use a temp name by default
	kase.setParent(mesher)
	c.forward()
	if !c.currentIs(tokenLeftBrace) { // case <name> {}, case <name> <cond> {}, case <cond> {}
		if c.currentIs(tokenString) { // case <name> {}, case <name> <cond> {}
			if caseName := c.current().text; caseName != "" {
				kase.setName(caseName) // change name
			}
			c.forward()
		}
		if !c.currentIs(tokenLeftBrace) { // case <name> <cond> {}
			c.parseCaseCond(kase)
			c.forwardExpect(tokenLeftBrace)
		}
	}
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c.parseAssign(current, kase)
			continue
		}
		if current.kind != tokenIdentifier {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compQUICDealer:
			c.parseQUICDealer(current, mesher, kase)
		case compQUICEditor:
			c.parseQUICEditor(current, mesher, kase)
		default:
			panic(fmt.Errorf("unknown component '%s' in quicCase\n", current.text))
		}
	}
}
func (c *config) parseTCPSMesher(stage *Stage) { // tcpsMesher <name> {}
	mesherName := c.forwardExpect(tokenString)
	mesher := stage.createTCPSMesher(mesherName.text)
	mesher.setParent(stage)
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c.parseAssign(current, mesher)
			continue
		}
		if current.kind != tokenIdentifier {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in tcpsMesher\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compTCPSDealer:
			c.parseTCPSDealer(current, mesher, nil)
		case compTCPSEditor:
			c.parseTCPSEditor(current, mesher, nil)
		case compCase:
			c.parseTCPSCase(mesher)
		default:
			panic(fmt.Errorf("unknown component '%s' in tcpsMesher\n", current.text))
		}
	}
}
func (c *config) parseTCPSDealer(sign *token, mesher *TCPSMesher, kase *tcpsCase) { // tttDealer <name> {}, tttDealer {}
	parseComponent1(c, sign, mesher, mesher.createDealer, kase, kase.addDealer)
}
func (c *config) parseTCPSEditor(sign *token, mesher *TCPSMesher, kase *tcpsCase) { // tttEditor <name> {}, tttEditor {}
	parseComponent1(c, sign, mesher, mesher.createEditor, kase, kase.addEditor)
}
func (c *config) parseTCPSCase(mesher *TCPSMesher) { // case <name> {}, case <name> <cond> {}, case <cond> {}, case {}
	kase := mesher.createCase(c.newName()) // use a temp name by default
	kase.setParent(mesher)
	c.forward()
	if !c.currentIs(tokenLeftBrace) { // case <name> {}, case <name> <cond> {}, case <cond> {}
		if c.currentIs(tokenString) { // case <name> {}, case <name> <cond> {}
			if caseName := c.current().text; caseName != "" {
				kase.setName(caseName) // change name
			}
			c.forward()
		}
		if !c.currentIs(tokenLeftBrace) { // case <name> <cond> {}
			c.parseCaseCond(kase)
			c.forwardExpect(tokenLeftBrace)
		}
	}
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c.parseAssign(current, kase)
			continue
		}
		if current.kind != tokenIdentifier {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compTCPSDealer:
			c.parseTCPSDealer(current, mesher, kase)
		case compTCPSEditor:
			c.parseTCPSEditor(current, mesher, kase)
		default:
			panic(fmt.Errorf("unknown component '%s' in quicCase\n", current.text))
		}
	}
}
func (c *config) parseUDPSMesher(stage *Stage) { // udpsMesher <name> {}
	mesherName := c.forwardExpect(tokenString)
	mesher := stage.createUDPSMesher(mesherName.text)
	mesher.setParent(stage)
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c.parseAssign(current, mesher)
			continue
		}
		if current.kind != tokenIdentifier {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in udpsMesher\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compUDPSDealer:
			c.parseUDPSDealer(current, mesher, nil)
		case compUDPSEditor:
			c.parseUDPSEditor(current, mesher, nil)
		case compCase:
			c.parseUDPSCase(mesher)
		default:
			panic(fmt.Errorf("unknown component '%s' in udpsMesher\n", current.text))
		}
	}
}
func (c *config) parseUDPSDealer(sign *token, mesher *UDPSMesher, kase *udpsCase) { // uuuDealer <name> {}, uuuDealer {}
	parseComponent1(c, sign, mesher, mesher.createDealer, kase, kase.addDealer)
}
func (c *config) parseUDPSEditor(sign *token, mesher *UDPSMesher, kase *udpsCase) { // uuuEditor <name> {}, uuuEditor {}
	parseComponent1(c, sign, mesher, mesher.createEditor, kase, kase.addEditor)
}
func (c *config) parseUDPSCase(mesher *UDPSMesher) { // case <name> {}, case <name> <cond> {}, case <cond> {}, case {}
	kase := mesher.createCase(c.newName()) // use a temp name by default
	kase.setParent(mesher)
	c.forward()
	if !c.currentIs(tokenLeftBrace) { // case <name> {}, case <name> <cond> {}, case <cond> {}
		if c.currentIs(tokenString) { // case <name> {}, case <name> <cond> {}
			if caseName := c.current().text; caseName != "" {
				kase.setName(caseName) // change name
			}
			c.forward()
		}
		if !c.currentIs(tokenLeftBrace) { // case <name> <cond> {}
			c.parseCaseCond(kase)
			c.forwardExpect(tokenLeftBrace)
		}
	}
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c.parseAssign(current, kase)
			continue
		}
		if current.kind != tokenIdentifier {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compUDPSDealer:
			c.parseUDPSDealer(current, mesher, kase)
		case compUDPSEditor:
			c.parseUDPSEditor(current, mesher, kase)
		default:
			panic(fmt.Errorf("unknown component '%s' in quicCase\n", current.text))
		}
	}
}
func (c *config) parseCaseCond(kase interface{ setInfo(info any) }) {
	variable := c.expect(tokenVariable)
	c.forward()
	if c.currentIs(tokenFSCheck) {
		panic(errors.New("config error: fs check is not allowed in case"))
	}
	cond := caseCond{varCode: variable.info}
	compare := c.expect(tokenCompare)
	patterns := []string{}
	if current := c.forward(); current.kind == tokenString {
		patterns = append(patterns, current.text)
	} else if current.kind == tokenLeftParen { // (
		for { // each element
			current = c.forward()
			if current.kind == tokenRightParen { // )
				break
			} else if current.kind == tokenString {
				patterns = append(patterns, current.text)
			} else {
				panic(errors.New("config error: only strings are allowed in cond"))
			}
			current = c.forward()
			if current.kind == tokenRightParen { // )
				break
			} else if current.kind != tokenComma {
				panic(errors.New("config error: bad string list in cond"))
			}
		}
	} else {
		panic(errors.New("config error: bad cond pattern"))
	}
	cond.patterns = patterns
	cond.compare = compare.text
	kase.setInfo(cond)
}
func (c *config) parseStater(sign *token, stage *Stage) { // xxxStater <name> {}
	parseComponent0(c, sign, stage, stage.createStater)
}
func (c *config) parseCacher(sign *token, stage *Stage) { // xxxCacher <name> {}
	parseComponent0(c, sign, stage, stage.createCacher)
}
func (c *config) parseApp(sign *token, stage *Stage) { // app <name> {}
	appName := c.forwardExpect(tokenString)
	app := stage.createApp(appName.text)
	app.setParent(stage)
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c.parseAssign(current, app)
			continue
		}
		if current.kind != tokenIdentifier {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in app\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compHandlet:
			c.parseHandlet(current, app, nil)
		case compReviser:
			c.parseReviser(current, app, nil)
		case compSocklet:
			c.parseSocklet(current, app, nil)
		case compRule:
			c.parseRule(app)
		default:
			panic(fmt.Errorf("unknown component '%s' in app\n", current.text))
		}
	}
}
func (c *config) parseHandlet(sign *token, app *App, rule *Rule) { // xxxHandlet <name> {}, xxxHandlet {}
	parseComponent2(c, sign, app, app.createHandlet, rule, rule.addHandlet)
}
func (c *config) parseReviser(sign *token, app *App, rule *Rule) { // xxxReviser <name> {}, xxxReviser {}
	parseComponent2(c, sign, app, app.createReviser, rule, rule.addReviser)
}
func (c *config) parseSocklet(sign *token, app *App, rule *Rule) { // xxxSocklet <name> {}, xxxSocklet {}
	parseComponent2(c, sign, app, app.createSocklet, rule, rule.addSocklet)
}
func (c *config) parseRule(app *App) { // rule <name> {}, rule <name> <cond> {}, rule <cond> {}, rule {}
	rule := app.createRule(c.newName()) // use a temp name by default
	rule.setParent(app)
	c.forward()
	if !c.currentIs(tokenLeftBrace) { // rule <name> {}, rule <name> <cond> {}, rule <cond> {}
		if c.currentIs(tokenString) { // rule <name> {}, rule <name> <cond> {}
			if ruleName := c.current().text; ruleName != "" {
				rule.setName(ruleName) // change name
			}
			c.forward()
		}
		if !c.currentIs(tokenLeftBrace) { // rule <name> <cond> {}
			c.parseRuleCond(rule)
			c.forwardExpect(tokenLeftBrace)
		}
	}
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c.parseAssign(current, rule)
			continue
		}
		if current.kind != tokenIdentifier {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in rule\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compHandlet:
			c.parseHandlet(current, app, rule)
		case compReviser:
			c.parseReviser(current, app, rule)
		case compSocklet:
			c.parseSocklet(current, app, rule)
		default:
			panic(fmt.Errorf("config error: unknown component %s=%s (in line %d) in rule\n", current.name(), current.text, current.line))
		}
	}
}
func (c *config) parseRuleCond(rule *Rule) {
	variable := c.expect(tokenVariable)
	c.forward()
	cond := ruleCond{varCode: variable.info}
	var compare *token
	if c.currentIs(tokenFSCheck) {
		if variable.text != "path" {
			panic(fmt.Errorf("config error: only path is allowed to test against file system, but got %s\n", variable.text))
		}
		compare = c.current()
	} else {
		compare = c.expect(tokenCompare)
		patterns := []string{}
		if current := c.forward(); current.kind == tokenString {
			patterns = append(patterns, current.text)
		} else if current.kind == tokenLeftParen { // (
			for { // each element
				current = c.forward()
				if current.kind == tokenRightParen { // )
					break
				} else if current.kind == tokenString {
					patterns = append(patterns, current.text)
				} else {
					panic(errors.New("config error: only strings are allowed in cond"))
				}
				current = c.forward()
				if current.kind == tokenRightParen { // )
					break
				} else if current.kind != tokenComma {
					panic(errors.New("config error: bad string list in cond"))
				}
			}
		} else {
			panic(errors.New("config error: bad cond pattern"))
		}
		cond.patterns = patterns
	}
	cond.compare = compare.text
	rule.setInfo(cond)
}
func (c *config) parseSvc(sign *token, stage *Stage) { // svc <name> {}
	svcName := c.forwardExpect(tokenString)
	svc := stage.createSvc(svcName.text)
	svc.setParent(stage)
	c.forward()
	c.parseLeaf(svc)
}
func (c *config) parseServer(sign *token, stage *Stage) { // xxxServer <name> {}
	parseComponent0(c, sign, stage, stage.createServer)
}
func (c *config) parseCronjob(sign *token, stage *Stage) { // xxxCronjob {}
	cronjob := stage.createCronjob(sign.text)
	cronjob.setParent(stage)
	c.forward()
	c.parseLeaf(cronjob)
}

func (c *config) parseLeaf(component Component) {
	c.expect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c.parseAssign(current, component)
			continue
		}
		panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in component\n", current.name(), current.text, current.line))
	}
}
func (c *config) parseAssign(prop *token, component Component) {
	if c.nextIs(tokenLeftBrace) { // {
		panic(fmt.Errorf("config error: unknown component '%s' (in line %d)\n", prop.text, prop.line))
	}
	c.forwardExpect(tokenEqual) // =
	current := c.forward()
	value := Value{line: current.line, file: current.file}
	c.parseValue(component, prop.text, &value)
	component.setProp(prop.text, value)
}

func (c *config) parseValue(component Component, prop string, value *Value) {
	current := c.current()
	switch current.kind {
	case tokenBool:
		value.kind, value.data = tokenBool, current.text == "true"
	case tokenInteger:
		last := current.text[len(current.text)-1]
		if byteIsDigit(last) {
			n64, err := strconv.ParseInt(current.text, 10, 64)
			if err != nil {
				panic(fmt.Errorf("config error: bad integer %s\n", current.text))
			}
			if n64 < 0 {
				panic(errors.New("config error: negative integers are not allowed"))
			}
			value.data = n64
		} else {
			size, err := strconv.ParseInt(current.text[:len(current.text)-1], 10, 64)
			if err != nil {
				panic(fmt.Errorf("config error: bad size %s\n", current.text))
			}
			if size < 0 {
				panic(errors.New("config error: negative sizes are not allowed"))
			}
			switch current.text[len(current.text)-1] {
			case 'K':
				size *= K
			case 'M':
				size *= M
			case 'G':
				size *= G
			case 'T':
				size *= T
			}
			value.data = size
		}
		value.kind = tokenInteger
	case tokenString:
		value.kind, value.data = tokenString, current.text
	case tokenDuration:
		last := len(current.text) - 1
		n, err := strconv.ParseInt(current.text[:last], 10, 64)
		if err != nil {
			panic(fmt.Errorf("config error: bad duration %s\n", current.text))
		}
		if n < 0 {
			panic(errors.New("config error: negative durations are not allowed"))
		}
		var d time.Duration
		switch current.text[last] {
		case 's':
			d = time.Duration(n) * time.Second
		case 'm':
			d = time.Duration(n) * time.Minute
		case 'h':
			d = time.Duration(n) * time.Hour
		case 'd':
			d = time.Duration(n) * 24 * time.Hour
		}
		value.kind, value.data = tokenDuration, d
	case tokenLeftParen: // (...)
		c.parseList(component, prop, value)
	case tokenLeftBracket: // [...]
		c.parseDict(component, prop, value)
	case tokenProperty: // .property
		if propRef := current.text; prop == "" || prop == propRef {
			panic(errors.New("config error: cannot refer to self"))
		} else if valueRef, ok := component.Find(propRef); !ok {
			panic(fmt.Errorf("config error: refer to a prop that doesn't exist in line %d\n", current.line))
		} else {
			value.kind, value.data = valueRef.kind, valueRef.data
		}
	default:
		panic(fmt.Errorf("config error: expect a value, but get token %s=%s (in line %d)\n", current.name(), current.text, current.line))
	}

	if value.kind != tokenString {
		// Currently only strings can be concatenated
		return
	}

	for {
		if !c.nextIs(tokenPlus) { // any concatenations?
			break
		}
		// Yes.
		c.forward() // +
		current = c.forward()
		str := Value{line: current.line, file: current.file}
		isString := false
		if c.currentIs(tokenString) {
			isString = true
			c.parseValue(component, prop, &str)
		} else if c.currentIs(tokenProperty) {
			if propRef := current.text; prop == "" || prop == propRef {
				panic(errors.New("config error: cannot refer to self"))
			} else if valueRef, ok := component.Find(propRef); !ok {
				panic(errors.New("config error: refere to a prop that doesn't exist"))
			} else {
				str.kind, str.data = valueRef.kind, valueRef.data
				if str.kind == tokenString {
					isString = true
				}
			}
		}
		if isString {
			value.data = value.data.(string) + str.data.(string)
		} else {
			panic(errors.New("config error: cannot concat string with other types. token=" + c.current().text))
		}
	}
}
func (c *config) parseList(component Component, prop string, value *Value) {
	list := []Value{}
	c.expect(tokenLeftParen) // (
	for {
		current := c.forward()
		if current.kind == tokenRightParen { // )
			break
		}
		elem := Value{line: current.line, file: current.file}
		c.parseValue(component, prop, &elem)
		list = append(list, elem)
		current = c.forward()
		if current.kind == tokenRightParen { // )
			break
		} else if current.kind != tokenComma { // ,
			panic(fmt.Errorf("config error: bad list in line %d\n", current.line))
		}
	}
	value.kind, value.data = tokenList, list
}
func (c *config) parseDict(component Component, prop string, value *Value) {
	dict := make(map[string]Value)
	c.expect(tokenLeftBracket) // [
	for {
		current := c.forward()
		if current.kind == tokenRightBracket { // ]
			break
		}
		k := c.expect(tokenString)  // k
		c.forwardExpect(tokenColon) // :
		current = c.forward()       // v
		v := Value{line: current.line, file: current.file}
		c.parseValue(component, prop, &v)
		dict[k.text] = v
		current = c.forward()
		if current.kind == tokenRightBracket { // ]
			break
		} else if current.kind != tokenComma { // ,
			panic(fmt.Errorf("config error: bad dict in line %d\n", current.line))
		}
	}
	value.kind, value.data = tokenDict, dict
}

func parseComponent0[T Component](c *config, sign *token, stage *Stage, create func(sign string, name string) T) { // backend, stater, cacher, server
	name := c.forwardExpect(tokenString)
	component := create(sign.text, name.text)
	component.setParent(stage)
	c.forward()
	c.parseLeaf(component)
}
func parseComponent1[M Component, T Component, C any](c *config, sign *token, mesher M, create func(sign string, name string) T, kase *C, assign func(T)) { // dealer, editor
	name := sign.text
	if current := c.forward(); current.kind == tokenString {
		name = current.text
		c.forward()
	} else if kase != nil { // in case
		name = c.newName()
	}
	component := create(sign.text, name)
	component.setParent(mesher)
	if kase != nil { // in case
		assign(component)
	}
	c.parseLeaf(component)
}
func parseComponent2[T Component](c *config, sign *token, app *App, create func(sign string, name string) T, rule *Rule, assign func(T)) { // handlet, reviser, socklet
	name := sign.text
	if current := c.forward(); current.kind == tokenString {
		name = current.text
		c.forward()
	} else if rule != nil { // in rule
		name = c.newName()
	}
	component := create(sign.text, name)
	component.setParent(app)
	if rule != nil { // in rule
		assign(component)
	}
	c.parseLeaf(component)
}

// caseCond is the case condition.
type caseCond struct {
	varCode  int16    // see varCodes
	compare  string   // ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~
	patterns []string // ...
}

// ruleCond is the rule condition.
type ruleCond struct {
	varCode   int16    // see varCodes
	logicType int8     // 0:no-logic 1:and 2:or
	compType  int8     // todo, undefined. for fast comparison
	compare   string   // ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~, -f, -d, -e, -D, -E, !f, !d, !e
	patterns  []string // ("GET", "POST"), ("https"), ("abc.com"), ("/hello", "/world")
}

// lexer
type lexer struct {
	index int
	limit int
	text  string // the config text
	base  string
	file  string
}

func (l *lexer) scanText(text string) []token {
	l.text = text
	return l.scan()
}
func (l *lexer) scanFile(base string, file string) []token {
	l.base, l.file = base, file
	return l.scan()
}

func (l *lexer) scan() []token {
	if l.file != "" {
		l.text = l.load(l.base, l.file)
	}
	l.index, l.limit = 0, len(l.text)
	var tokens []token
	line := int32(1)
	for l.index < l.limit {
		from := l.index
		switch b := l.text[l.index]; b {
		case ' ', '\t', '\r': // blank, ignore
			l.index++
		case '\n': // new line
			line++
			l.index++
		case '#': // shell comment
			l.nextUntil('\n')
		case '/': // line comment or stream comment
			if c := l.mustNext(); c == '/' { // line comment
				l.nextUntil('\n')
			} else if c == '*' { // stream comment
				l.index++
				for l.index < l.limit {
					if d := l.text[l.index]; d == '/' && l.text[l.index-1] == '*' {
						break
					} else {
						if d == '\n' {
							line++
						}
						l.index++
					}
				}
				l.checkEOF()
				l.index++
			} else {
				panic(fmt.Errorf("lexer: unknown character %c (ascii %v) in line %d (%s)\n", b, b, line, l.file))
			}
		case '=': // = or ==
			if l.index++; l.index < l.limit && l.text[l.index] == '=' { // ==
				tokens = append(tokens, token{tokenCompare, 0, line, l.file, "=="})
				l.index++
			} else { // =
				tokens = append(tokens, token{tokenEqual, 0, line, l.file, "="})
			}
		case '"', '`': // "string" or `string`
			s := l.text[l.index] // " or `
			l.index++
			l.nextUntil(s) // " or `
			l.checkEOF()
			tokens = append(tokens, token{tokenString, 0, line, l.file, l.text[from+1 : l.index]})
			l.index++
		case '<': // <includedFile>
			if l.base == "" {
				panic(errors.New("lexer: include is not allowed in text mode"))
			} else {
				l.index++
				l.nextUntil('>')
				l.checkEOF()
				file := l.text[from+1 : l.index]
				l.index++
				var ll lexer
				tokens = append(tokens, ll.scanFile(l.base, file)...)
			}
		case '%': // %constant
			l.nextAlnums()
			tokens = append(tokens, token{tokenConstant, 0, line, l.file, l.text[from+1 : l.index]})
		case '.': // .property
			l.nextAlnums()
			tokens = append(tokens, token{tokenProperty, 0, line, l.file, l.text[from+1 : l.index]})
		case '$': // $variable
			if !l.nextIs('=') {
				l.nextAlnums()
				tokens = append(tokens, token{tokenVariable, 0, line, l.file, l.text[from+1 : l.index]})
				break
			}
			fallthrough
		case '^', '*', '~': // ^=, $=, *=, ~=
			if l.mustNext() != '=' {
				panic(fmt.Errorf("lexer: unknown character %c (ascii %v) in line %d (%s)\n", b, b, line, l.file))
			}
			l.index++
			tokens = append(tokens, token{tokenCompare, 0, line, l.file, l.text[from:l.index]})
		case '-': // -f, -d, -e, -D, -E
			if c := l.mustNext(); c != 'f' && c != 'd' && c != 'e' && c != 'D' && c != 'E' {
				panic(fmt.Errorf("lexer: not a valid FSCHECK in line %d (%s)\n", line, l.file))
			}
			l.index++
			tokens = append(tokens, token{tokenFSCheck, 0, line, l.file, l.text[from:l.index]})
		case '!': // !=, !^, !$, !*, !~, !f, !d, !e
			if c := l.mustNext(); c == '=' || c == '^' || c == '$' || c == '*' || c == '~' {
				tokens = append(tokens, token{tokenCompare, 0, line, l.file, l.text[from : l.index+1]})
			} else if c == 'f' || c == 'd' || c == 'e' {
				tokens = append(tokens, token{tokenFSCheck, 0, line, l.file, l.text[from : l.index+1]})
			} else {
				panic(fmt.Errorf("lexer: not a valid COMPARE or FSCHECK in line %d (%s)\n", line, l.file))
			}
			l.index++
		case '&': // &&
			if l.mustNext() != '&' {
				panic(fmt.Errorf("lexer: not a valid AND in line %d (%s)\n", line, l.file))
			}
			tokens = append(tokens, token{tokenAND, 0, line, l.file, "&&"})
			l.index++
		case '|': // ||
			if l.mustNext() != '|' {
				panic(fmt.Errorf("lexer: not a valid OR in line %d (%s)\n", line, l.file))
			}
			tokens = append(tokens, token{tokenOR, 0, line, l.file, "||"})
			l.index++
		default:
			if kind := soloKinds[b]; kind != 0 { // kind starts from 1
				tokens = append(tokens, token{kind, 0, line, l.file, soloTexts[b]})
				l.index++
			} else if byteIsAlpha(b) {
				l.nextAlnums()
				if identifier := l.text[from:l.index]; identifier == "true" || identifier == "false" {
					tokens = append(tokens, token{tokenBool, 0, line, l.file, identifier})
				} else {
					tokens = append(tokens, token{tokenIdentifier, 0, line, l.file, identifier})
				}
			} else if byteIsDigit(b) {
				l.nextDigits()
				digits := true
				if l.index < l.limit {
					switch l.text[l.index] {
					case 's', 'm', 'h', 'd':
						digits = false
						l.index++
						tokens = append(tokens, token{tokenDuration, 0, line, l.file, l.text[from:l.index]})
					case 'K', 'M', 'G', 'T':
						digits = false
						l.index++
						tokens = append(tokens, token{tokenInteger, 0, line, l.file, l.text[from:l.index]})
					}
				}
				if digits {
					tokens = append(tokens, token{tokenInteger, 0, line, l.file, l.text[from:l.index]})
				}
			} else {
				panic(fmt.Errorf("lexer: unknown character %c (ascii %v) in line %d (%s)\n", b, b, line, l.file))
			}
		}
	}
	return tokens
}

func (l *lexer) nextUntil(b byte) {
	if i := strings.IndexByte(l.text[l.index:], b); i == -1 {
		l.index = l.limit
	} else {
		l.index += i
	}
}
func (l *lexer) nextIs(b byte) bool {
	if next := l.index + 1; next != l.limit {
		return l.text[next] == b
	}
	return false
}
func (l *lexer) mustNext() byte {
	l.index++
	l.checkEOF()
	return l.text[l.index]
}
func (l *lexer) checkEOF() {
	if l.index == l.limit {
		panic(errors.New("lexer: unexpected eof"))
	}
}
func (l *lexer) nextAlnums() {
	for l.index++; l.index < l.limit && byteIsAlnum(l.text[l.index]); l.index++ {
	}
}
func (l *lexer) nextDigits() {
	for l.index++; l.index < l.limit && byteIsDigit(l.text[l.index]); l.index++ {
	}
}

func (l *lexer) load(base string, file string) string {
	if true { // TODO
		return l._loadLFS(base, file)
	} else {
		return l._loadURL(base, file)
	}
}
func (l *lexer) _loadLFS(base string, file string) string {
	path := file
	if file[0] != '/' {
		if base[len(base)-1] == '/' {
			path = base + file
		} else {
			path = base + "/" + file
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(data)
}
func (l *lexer) _loadURL(base string, file string) string {
	// TODO
	return ""
}

// token is a token in config file.
type token struct { // 40 bytes
	kind int16  // tokenXXX
	info int16  // comp for identifiers, or code for variables
	line int32  // at line number
	file string // file path
	text string // text literal
}

func (t token) name() string { return tokenNames[t.kind] }

const ( // token list. if you change this list, change in tokenNames too.
	// Identifier
	tokenIdentifier = 1 + iota // stage, apps, httpxServer, ...
	// Operators
	tokenLeftBrace    // {
	tokenRightBrace   // }
	tokenLeftBracket  // [
	tokenRightBracket // ]
	tokenLeftParen    // (
	tokenRightParen   // )
	tokenComma        // ,
	tokenColon        // :
	tokenPlus         // +
	tokenEqual        // =
	tokenCompare      // ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~
	tokenFSCheck      // -f, -d, -e, -D, -E, !f, !d, !e
	tokenAND          // &&
	tokenOR           // ||
	// Constants
	tokenConstant // %baseDir, %logsDir, %tempDir, %varsDir
	// Properties
	tokenProperty // .listen, .maxSize, ...
	// Variables
	tokenVariable // $method, $path, ...
	// Values
	tokenBool     // true, false
	tokenInteger  // 123, 16K, 256M, ...
	tokenString   // "", "abc", `def`, ...
	tokenDuration // 1s, 2m, 3h, 4d, ...
	tokenList     // lists: (...)
	tokenDict     // dicts: [...]
)

var tokenNames = [...]string{ // token names. if you change this list, change in token list too.
	// Identifier
	tokenIdentifier: "identifier",
	// Operators
	tokenLeftBrace:    "leftBrace",
	tokenRightBrace:   "rightBrace",
	tokenLeftBracket:  "leftBracket",
	tokenRightBracket: "rightBracket",
	tokenLeftParen:    "leftParen",
	tokenRightParen:   "rightParen",
	tokenComma:        "comma",
	tokenColon:        "colon",
	tokenPlus:         "plus",
	tokenEqual:        "equal",
	tokenCompare:      "compare",
	tokenFSCheck:      "fsCheck",
	tokenAND:          "and",
	tokenOR:           "or",
	// Constants
	tokenConstant: "constant",
	// Properties
	tokenProperty: "property",
	// Variables
	tokenVariable: "variable",
	// Value literals
	tokenBool:     "bool",
	tokenInteger:  "integer",
	tokenString:   "string",
	tokenDuration: "duration",
	tokenList:     "list",
	tokenDict:     "dict",
}

var ( // solos
	soloKinds = [256]int16{ // keep sync with soloTexts
		'{': tokenLeftBrace,
		'}': tokenRightBrace,
		'[': tokenLeftBracket,
		']': tokenRightBracket,
		'(': tokenLeftParen,
		')': tokenRightParen,
		',': tokenComma,
		':': tokenColon,
		'+': tokenPlus,
	}
	soloTexts = [...]string{ // keep sync with soloKinds
		'{': "{",
		'}': "}",
		'[': "[",
		']': "]",
		'(': "(",
		')': ")",
		',': ",",
		':': ":",
		'+': "+",
	}
)

// Value is a value in config file.
type Value struct { // 40 bytes
	kind int16  // tokenXXX in values
	line int32  // at line number
	file string // in file
	data any    // bools, integers, strings, durations, lists, and dicts
}

func (v *Value) IsBool() bool     { return v.kind == tokenBool }
func (v *Value) IsInteger() bool  { return v.kind == tokenInteger }
func (v *Value) IsString() bool   { return v.kind == tokenString }
func (v *Value) IsDuration() bool { return v.kind == tokenDuration }
func (v *Value) IsList() bool     { return v.kind == tokenList }
func (v *Value) IsDict() bool     { return v.kind == tokenDict }

func (v *Value) Bool() (b bool, ok bool) {
	b, ok = v.data.(bool)
	return
}
func (v *Value) Int64() (i64 int64, ok bool) {
	i64, ok = v.data.(int64)
	return
}
func toInt[T ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](v *Value) (i T, ok bool) {
	i64, ok := v.Int64()
	i = T(i64)
	if ok && int64(i) != i64 {
		ok = false
	}
	return
}
func (v *Value) Uint32() (u32 uint32, ok bool) { return toInt[uint32](v) }
func (v *Value) Int32() (i32 int32, ok bool)   { return toInt[int32](v) }
func (v *Value) Int16() (i16 int16, ok bool)   { return toInt[int16](v) }
func (v *Value) Int8() (i8 int8, ok bool)      { return toInt[int8](v) }
func (v *Value) Int() (i int, ok bool)         { return toInt[int](v) }
func (v *Value) String() (s string, ok bool) {
	s, ok = v.data.(string)
	return
}
func (v *Value) Bytes() (p []byte, ok bool) {
	if s, isString := v.String(); isString {
		return []byte(s), true
	}
	return
}
func (v *Value) Duration() (d time.Duration, ok bool) {
	d, ok = v.data.(time.Duration)
	return
}
func (v *Value) List() (list []Value, ok bool) {
	list, ok = v.data.([]Value)
	return
}
func (v *Value) ListN(n int) (list []Value, ok bool) {
	list, ok = v.data.([]Value)
	if ok && n >= 0 && len(list) != n {
		ok = false
	}
	return
}
func (v *Value) StringList() (list []string, ok bool) {
	l, ok := v.data.([]Value)
	if ok {
		for _, value := range l {
			if s, isString := value.String(); isString {
				list = append(list, s)
			}
		}
	}
	return
}
func (v *Value) BytesList() (list [][]byte, ok bool) {
	l, ok := v.data.([]Value)
	if ok {
		for _, value := range l {
			if s, isString := value.String(); isString {
				list = append(list, []byte(s))
			}
		}
	}
	return
}
func (v *Value) StringListN(n int) (list []string, ok bool) {
	l, ok := v.data.([]Value)
	if !ok {
		return
	}
	if n >= 0 && len(l) != n {
		ok = false
		return
	}
	for _, value := range l {
		if s, ok := value.String(); ok {
			list = append(list, s)
		}
	}
	return
}
func (v *Value) Dict() (dict map[string]Value, ok bool) {
	dict, ok = v.data.(map[string]Value)
	return
}
func (v *Value) StringDict() (dict map[string]string, ok bool) {
	d, ok := v.data.(map[string]Value)
	if ok {
		dict = make(map[string]string)
		for name, value := range d {
			if s, ok := value.String(); ok {
				dict[name] = s
			}
		}
	}
	return
}
