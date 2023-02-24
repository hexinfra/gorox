// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Configuration.

package internal

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func ApplyText(text string) (*Stage, error) {
	c := _getConfig()
	return c.applyText(text)
}
func ApplyFile(base string, file string) (*Stage, error) {
	c := _getConfig()
	return c.applyFile(base, file)
}
func _getConfig() (c config) {
	constants := map[string]string{
		"baseDir": BaseDir(),
		"dataDir": DataDir(),
		"logsDir": LogsDir(),
		"tempDir": TempDir(),
	}
	c.init(constants, varCodes, signedComps)
	return
}

const ( // comp list
	compStage      = 1 + iota // stage
	compFixture               // clock, filesys, resolv, http1, http2, http3, quic, tcps, udps, unix
	compUniture               // ...
	compBackend               // HTTP1Backend, HTTP2Backend, HTTP3Backend, QUICBackend, TCPSBackend, UDPSBackend, UnixBackend
	compQUICMesher            // quicMesher
	compQUICDealet            // quicProxy, ...
	compQUICEditor            // ...
	compTCPSMesher            // tcpsMesher
	compTCPSDealet            // tcpsProxy, ...
	compTCPSEditor            // ...
	compUDPSMesher            // udpsMesher
	compUDPSDealet            // udpsProxy, ...
	compUDPSEditor            // ...
	compCase                  // case
	compStater                // localStater, redisStater, ...
	compCacher                // localCacher, redisCacher, ...
	compApp                   // app
	compHandlet               // static, ...
	compReviser               // gzipReviser, wrapReviser, ...
	compSocklet               // helloSocklet, ...
	compRule                  // rule
	compSvc                   // svc
	compServer                // httpxServer, echoServer, ...
	compCronjob               // cleanCronjob, statCronjob, ...
)

var varCodes = map[string]int16{ // predefined variables for config
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

	// http request vars. keep sync with httpRequestVariables in server_http.go
	"method":      0, // GET, POST, ...
	"scheme":      1, // http, https
	"authority":   2, // example.com, example.org:8080
	"hostname":    3, // example.com, example.org
	"colonPort":   4, // :80, :8080
	"path":        5, // /abc, /def/
	"uri":         6, // /abc?x=y, /%cc%dd?y=z&z=%ff
	"encodedPath": 7, // /abc, /%cc%dd
	"queryString": 8, // ?x=y, ?y=z&z=%ff
	"contentType": 9, // text/html; charset=utf-8
}

var signedComps = map[string]int16{ // static comps. more dynamic comps are signed using signComp() below
	"stage":      compStage,
	"quicMesher": compQUICMesher,
	"tcpsMesher": compTCPSMesher,
	"udpsMesher": compUDPSMesher,
	"case":       compCase,
	"app":        compApp,
	"rule":       compRule,
	"svc":        compSvc,
}

func signComp(sign string, comp int16) {
	if have, signed := signedComps[sign]; signed {
		BugExitf("conflicting sign: comp=%d sign=%s\n", have, sign)
	}
	signedComps[sign] = comp
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
	return c.apply()
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
	return c.apply()
}

func (c *config) show() {
	for _, token := range c.tokens {
		fmt.Printf("kind=%16s info=%2d line=%4d file=%s    %s", token.name(), token.info, token.line, token.file, token.text)
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

func (c *config) current() token            { return c.tokens[c.index] }
func (c *config) currentIs(kind int16) bool { return c.tokens[c.index].kind == kind }
func (c *config) nextIs(kind int16) bool {
	if c.index == c.limit {
		return false
	}
	return c.tokens[c.index+1].kind == kind
}
func (c *config) expect(kind int16) token {
	current := c.tokens[c.index]
	if current.kind != kind {
		panic(fmt.Errorf("config: expect %s, but get %s=%s (in line %d)\n", tokenNames[kind], tokenNames[current.kind], current.text, current.line))
	}
	return current
}
func (c *config) forwardExpect(kind int16) token {
	if c.index++; c.index == c.limit {
		panic(errors.New("config: unexpected EOF"))
	}
	return c.expect(kind)
}
func (c *config) forward() token {
	if c.index++; c.index == c.limit {
		panic(errors.New("config: unexpected EOF"))
	}
	return c.tokens[c.index]
}
func (c *config) newName() string {
	c.counter++
	return strconv.Itoa(c.counter)
}

func (c *config) apply() (stage *Stage, err error) {
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
		} else if current.kind == tokenIdentifier {
			switch current.text {
			case "fixtures":
				c.parseContainer0(compFixture, c.parseFixture, current.text, stage)
			case "unitures":
				c.parseContainer0(compUniture, c.parseUniture, current.text, stage)
			case "backends":
				c.parseContainer0(compBackend, c.parseBackend, current.text, stage)
			case "meshers":
				c.parseMeshers(stage)
			case "staters":
				c.parseContainer0(compStater, c.parseStater, current.text, stage)
			case "cachers":
				c.parseContainer0(compCacher, c.parseCacher, current.text, stage)
			case "apps":
				c.parseContainer0(compApp, c.parseApp, current.text, stage)
			case "svcs":
				c.parseContainer0(compSvc, c.parseSvc, current.text, stage)
			case "servers":
				c.parseContainer0(compServer, c.parseServer, current.text, stage)
			case "cronjobs":
				c.parseContainer0(compCronjob, c.parseCronjob, current.text, stage)
			default:
				panic(fmt.Errorf("unknown container '%s' in stage\n", current.text))
			}
		} else {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in stage\n", current.name(), current.text, current.line))
		}
	}
}
func (c *config) parseContainer0(comp int16, parseComponent func(sign token, stage *Stage), compName string, stage *Stage) { // fixtures, unitures, backends, staters, cachers, apps, svcs, servers, cronjobs {}
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenIdentifier && current.info == comp {
			parseComponent(current, stage)
		} else {
			panic(errors.New("config error: only " + compName + " are allowed in " + compName))
		}
	}
}

func (c *config) parseFixture(sign token, stage *Stage) { // xxxFixture {}
	fixtureSign := sign.text
	fixture := stage.fixture(fixtureSign)
	if fixture == nil {
		panic(errors.New("config error: unknown fixture: " + fixtureSign))
	}
	fixture.setParent(stage)
	c.forward()
	c.parseAssigns(fixture)
}
func (c *config) parseUniture(sign token, stage *Stage) { // xxxUniture {}
	uniture := stage.createUniture(sign.text)
	uniture.setParent(stage)
	c.forward()
	c.parseAssigns(uniture)
}
func (c *config) parseBackend(sign token, stage *Stage) { // xxxBackend <name> {}
	parseComponent0(c, sign, stage, stage.createBackend)
}
func parseComponent0[T Component](c *config, sign token, stage *Stage, create func(sign string, name string) T) { // backend, stater, cacher, server
	name := c.forwardExpect(tokenString)
	component := create(sign.text, name.text)
	component.setParent(stage)
	c.forward()
	c.parseAssigns(component)
}
func (c *config) parseMeshers(stage *Stage) { // meshers {}
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenIdentifier {
			switch current.info {
			case compQUICMesher:
				c.parseQUICMesher(stage)
			case compTCPSMesher:
				c.parseTCPSMesher(stage)
			case compUDPSMesher:
				c.parseUDPSMesher(stage)
			default:
				panic(errors.New("config error: only quicMesher, tcpsMesher, and udpsMesher are allowed in meshers"))
			}
		} else {
			panic(errors.New("config error: only meshers are allowed in meshers"))
		}
	}
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
		} else if current.kind == tokenIdentifier {
			switch current.text {
			case "dealets":
				parseContainer1(c, mesher, compQUICDealet, c.parseQUICDealet, current.text)
			case "editors":
				parseContainer1(c, mesher, compQUICEditor, c.parseQUICEditor, current.text)
			case "cases":
				parseCases(c, mesher, c.parseQUICCase)
			default:
				panic(fmt.Errorf("unknown container '%s' in quicMesher\n", current.text))
			}
		} else {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in quicMesher\n", current.name(), current.text, current.line))
		}
	}
}
func (c *config) parseQUICDealet(sign token, mesher *QUICMesher, kase *quicCase) { // qqqDealet <name> {}, qqqDealet {}
	parseComponent1(c, sign, mesher, mesher.createDealet, kase, kase.addDealet)
}
func (c *config) parseQUICEditor(sign token, mesher *QUICMesher, kase *quicCase) { // qqqEditor <name> {}, qqqEditor {}
	parseComponent1(c, sign, mesher, mesher.createEditor, kase, kase.addEditor)
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
		} else if current.kind == tokenIdentifier {
			switch current.text {
			case "dealets":
				parseContainer1(c, mesher, compTCPSDealet, c.parseTCPSDealet, current.text)
			case "editors":
				parseContainer1(c, mesher, compTCPSEditor, c.parseTCPSEditor, current.text)
			case "cases":
				parseCases(c, mesher, c.parseTCPSCase)
			default:
				panic(fmt.Errorf("unknown container '%s' in tcpsMesher\n", current.text))
			}
		} else {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in tcpsMesher\n", current.name(), current.text, current.line))
		}
	}
}
func (c *config) parseTCPSDealet(sign token, mesher *TCPSMesher, kase *tcpsCase) { // tttDealet <name> {}, tttDealet {}
	parseComponent1(c, sign, mesher, mesher.createDealet, kase, kase.addDealet)
}
func (c *config) parseTCPSEditor(sign token, mesher *TCPSMesher, kase *tcpsCase) { // tttEditor <name> {}, tttEditor {}
	parseComponent1(c, sign, mesher, mesher.createEditor, kase, kase.addEditor)
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
		} else if current.kind == tokenIdentifier {
			switch current.text {
			case "dealets":
				parseContainer1(c, mesher, compUDPSDealet, c.parseUDPSDealet, current.text)
			case "editors":
				parseContainer1(c, mesher, compUDPSEditor, c.parseUDPSEditor, current.text)
			case "cases":
				parseCases(c, mesher, c.parseUDPSCase)
			default:
				panic(fmt.Errorf("unknown container '%s' in udpsMesher\n", current.text))
			}
		} else {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in udpsMesher\n", current.name(), current.text, current.line))
		}
	}
}
func (c *config) parseUDPSDealet(sign token, mesher *UDPSMesher, kase *udpsCase) { // uuuDealet <name> {}, uuuDealet {}
	parseComponent1(c, sign, mesher, mesher.createDealet, kase, kase.addDealet)
}
func (c *config) parseUDPSEditor(sign token, mesher *UDPSMesher, kase *udpsCase) { // uuuEditor <name> {}, uuuEditor {}
	parseComponent1(c, sign, mesher, mesher.createEditor, kase, kase.addEditor)
}
func parseContainer1[M Component, C any](c *config, mesher M, comp int16, parseComponent func(sign token, mesher M, kase *C), compName string) { // dealets, editors {}
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenIdentifier && current.info == comp {
			parseComponent(current, mesher, nil) // not in case
		} else {
			panic(errors.New("config error: only " + compName + " are allowed in " + compName))
		}
	}
}
func parseComponent1[M Component, T Component, C any](c *config, sign token, mesher M, create func(sign string, name string) T, kase *C, assign func(T)) { // dealet, editor
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
	c.parseAssigns(component)
}

func parseCases[M Component](c *config, mesher M, parseCase func(M)) { // cases {}
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenIdentifier && current.info == compCase {
			parseCase(mesher)
		} else {
			panic(errors.New("config error: only cases are allowed in cases"))
		}
	}
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
		} else if current.kind == tokenIdentifier && current.info == compQUICEditor {
			c.parseQUICEditor(current, mesher, kase)
		} else {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.name(), current.text, current.line))
		}
	}
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
		} else if current.kind == tokenIdentifier && current.info == compTCPSEditor {
			c.parseTCPSEditor(current, mesher, kase)
		} else {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.name(), current.text, current.line))
		}
	}
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
		} else if current.kind == tokenIdentifier && current.info == compUDPSEditor {
			c.parseUDPSEditor(current, mesher, kase)
		} else {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.name(), current.text, current.line))
		}
	}
}
func (c *config) parseCaseCond(kase interface{ setInfo(info any) }) {
	variable := c.expect(tokenVariable)
	c.forward()
	cond := caseCond{varCode: variable.info}
	var compare token
	if c.currentIs(tokenFSCheck) {
		panic(errors.New("config error: fs check is not allowed in case"))
	}
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
	cond.compare = compare.text
	kase.setInfo(cond)
}

func (c *config) parseStater(sign token, stage *Stage) { // xxxStater <name> {}
	parseComponent0(c, sign, stage, stage.createStater)
}

func (c *config) parseCacher(sign token, stage *Stage) { // xxxCacher <name> {}
	parseComponent0(c, sign, stage, stage.createCacher)
}

func (c *config) parseApp(sign token, stage *Stage) { // app <name> {}
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
		} else if current.kind == tokenIdentifier {
			switch current.text {
			case "handlets":
				c.parseContainer2(app, compHandlet, c.parseHandlet, current.text)
			case "revisers":
				c.parseContainer2(app, compReviser, c.parseReviser, current.text)
			case "socklets":
				c.parseContainer2(app, compSocklet, c.parseSocklet, current.text)
			case "rules":
				c.parseRules(app)
			default:
				panic(fmt.Errorf("unknown container '%s' in app\n", current.text))
			}
		} else {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in app\n", current.name(), current.text, current.line))
		}
	}
}
func (c *config) parseContainer2(app *App, comp int16, parseComponent func(sign token, app *App, rule *Rule), compName string) { // handlets, revisers, socklets {}
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenIdentifier && current.info == comp {
			parseComponent(current, app, nil) // not in rule
		} else {
			panic(errors.New("config error: only " + compName + " are allowed in " + compName))
		}
	}
}

func (c *config) parseHandlet(sign token, app *App, rule *Rule) { // xxxHandlet <name> {}, xxxHandlet {}
	parseComponent2(c, sign, app, app.createHandlet, rule, rule.addHandlet)
}
func (c *config) parseReviser(sign token, app *App, rule *Rule) { // xxxReviser <name> {}, xxxReviser {}
	parseComponent2(c, sign, app, app.createReviser, rule, rule.addReviser)
}
func (c *config) parseSocklet(sign token, app *App, rule *Rule) { // xxxSocklet <name> {}, xxxSocklet {}
	parseComponent2(c, sign, app, app.createSocklet, rule, rule.addSocklet)
}
func parseComponent2[T Component](c *config, sign token, app *App, create func(sign string, name string) T, rule *Rule, assign func(T)) { // handlet, reviser, socklet
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
	c.parseAssigns(component)
}

func (c *config) parseRules(app *App) { // rules {}
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenIdentifier && current.info == compRule {
			c.parseRule(app)
		} else {
			panic(errors.New("config error: only rules are allowed in rules"))
		}
	}
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
		} else if current.kind == tokenIdentifier {
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
		} else {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in rule\n", current.name(), current.text, current.line))
		}
	}
}
func (c *config) parseRuleCond(rule *Rule) {
	variable := c.expect(tokenVariable)
	c.forward()
	cond := ruleCond{varCode: variable.info}
	var compare token
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

func (c *config) parseSvc(sign token, stage *Stage) { // svc <name> {}
	svcName := c.forwardExpect(tokenString)
	svc := stage.createSvc(svcName.text)
	svc.setParent(stage)
	c.forward()
	c.parseAssigns(svc)
}

func (c *config) parseServer(sign token, stage *Stage) { // xxxServer <name> {}
	parseComponent0(c, sign, stage, stage.createServer)
}
func (c *config) parseCronjob(sign token, stage *Stage) { // xxxCronjob {}
	cronjob := stage.createCronjob(sign.text)
	cronjob.setParent(stage)
	c.forward()
	c.parseAssigns(cronjob)
}

func (c *config) parseAssigns(component Component) {
	c.expect(tokenLeftBrace) // {
	for {
		switch current := c.forward(); current.kind {
		case tokenProperty:
			c.parseAssign(current, component)
		case tokenRightBrace: // }
			return
		default:
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in component\n", current.name(), current.text, current.line))
		}
	}
}
func (c *config) parseAssign(prop token, component Component) {
	if c.nextIs(tokenLeftBrace) { // {
		panic(fmt.Errorf("config error: unknown component '%s' (in line %d)\n", prop.text, prop.line))
	}
	c.forwardExpect(tokenEqual) // =
	c.forward()
	var value Value
	c.parseValue(component, prop.text, &value)
	component.setProp(prop.text, value)
}

func (c *config) parseValue(component Component, prop string, value *Value) {
	current := c.current()
	switch current.kind {
	case tokenBool:
		*value = Value{tokenBool, current.text == "true"}
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
			*value = Value{tokenInteger, n64}
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
			*value = Value{tokenInteger, size}
		}
	case tokenString:
		*value = Value{tokenString, current.text}
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
		*value = Value{tokenDuration, d}
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
			*value = valueRef
		}
	default:
		panic(fmt.Errorf("config error: expect a value, but get token %s=%s (in line %d)\n", current.name(), current.text, current.line))
	}

	if value.kind != tokenString {
		// Currently only strings can be concatenated
		return
	}

	for {
		// Any concatenations?
		if !c.nextIs(tokenPlus) {
			// No
			break
		}
		// Yes.
		c.forward() // +
		current = c.forward()
		var str Value
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
				str = valueRef
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
		var v Value
		c.parseValue(component, prop, &v)
		list = append(list, v)
		current = c.forward()
		if current.kind == tokenRightParen { // )
			break
		} else if current.kind != tokenComma { // ,
			panic(fmt.Errorf("config error: bad list in line %d\n", current.line))
		}
	}
	value.kind = tokenList
	value.data = list
}
func (c *config) parseDict(component Component, prop string, value *Value) {
	dict := make(map[string]Value)
	c.expect(tokenLeftBracket) // [
	for {
		current := c.forward()
		if current.kind == tokenRightBracket { // ]
			break
		}
		k := c.expect(tokenString)
		c.forwardExpect(tokenColon) // :
		c.forward()
		var v Value
		c.parseValue(component, prop, &v)
		dict[k.text] = v
		current = c.forward()
		if current.kind == tokenRightBracket { // ]
			break
		} else if current.kind != tokenComma { // ,
			panic(fmt.Errorf("config error: bad dict in line %d\n", current.line))
		}
	}
	value.kind = tokenDict
	value.data = dict
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
		case ' ', '\r', '\t': // blank, ignore
			l.index++
		case '\n': // new line
			line++
			l.index++
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
			l.nextUntil(s)
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

// token
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
	tokenConstant // %baseDir, %dataDir, %logsDir, %tempDir
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

var soloKinds = [256]int16{ // keep sync with soloTexts
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
var soloTexts = [...]string{ // keep sync with soloKinds
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

// Value is a value in config file.
type Value struct {
	kind int16 // tokenXXX in values
	data any   // bools, integers, strings, durations, lists, and dicts
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
