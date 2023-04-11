// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Site.

package sitex

import (
	"bytes"
	. "github.com/hexinfra/gorox/hemi/internal"
	"os"
	"reflect"
)

// Site
type Site struct {
	name      string
	hostnames []string
	viewDir   string
	settings  map[string]string
	pack      reflect.Type
}

func (s *Site) show(req Request, resp Response, page string) {
	if html := s.load(req, s.viewDir+"/"+page+".html"); html == nil {
		resp.SendNotFound(nil)
	} else {
		resp.SendBytes(html)
	}
}
func (s *Site) load(req Request, htmlFile string) []byte {
	html, err := os.ReadFile(htmlFile)
	if err != nil {
		return nil
	}
	var subs [][]byte
	for {
		i := bytes.Index(html, htmlLL)
		if i == -1 {
			break
		}
		j := bytes.Index(html, htmlRR)
		if j < i {
			break
		}
		subs = append(subs, html[:i])
		i += len(htmlLL)
		token := string(html[i:j])
		if first := token[0]; first == '$' {
			switch token {
			case "$scheme":
				subs = append(subs, []byte(req.Scheme()))
			case "$colonPort":
				subs = append(subs, []byte(req.ColonPort()))
			case "$uri": // TODO: XSS
				subs = append(subs, []byte(req.URI()))
			default:
				// Do nothing
			}
		} else if first == '@' {
			subs = append(subs, []byte(s.settings[token[1:]]))
		} else {
			subs = append(subs, s.load(req, s.viewDir+"/"+token))
		}
		html = html[j+len(htmlRR):]
	}
	if len(subs) == 0 {
		return html
	}
	subs = append(subs, html)
	return bytes.Join(subs, nil)
}

var (
	htmlLL = []byte("{{")
	htmlRR = []byte("}}")
)

// Target
type Target struct {
	Site string            // front
	Path string            // /foo/bar
	Args map[string]string // a=b&c=d
}
