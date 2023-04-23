// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Access checkers allow limiting access to certain client addresses.

package access

import (
	"bytes"
	"net"
	"sort"
	"strings"

	. "github.com/hexinfra/gorox/hemi/internal"
)

const (
	rankIP   = 16
	rankCIDR = 8
	rankAll  = 4
)

func init() {
	RegisterHandlet("accessChecker", func(name string, stage *Stage, app *App) Handlet {
		h := new(accessChecker)
		h.onCreate(name, stage, app)
		return h
	})
}

// accessChecker
type accessChecker struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
	allow []string // allows access for the specified network or address.
	deny  []string // denies access for the specified network or address.

	allowRules []*ipRule
	denyRules  []*ipRule
}

func (h *accessChecker) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}

func (h *accessChecker) OnShutdown() {
	h.app.SubDone()
}

func (h *accessChecker) OnConfigure() {
	// allow
	h.ConfigureStringList("allow", &h.allow, func(rules []string) bool { return checkRule(rules) }, []string{"all"})
	// deny
	h.ConfigureStringList("deny", &h.deny, func(rules []string) bool {
		if !checkRule(rules) {
			return false
		}
		if checkRuleConflict(h.allow, rules) {
			// TODO: h.stage.logger hasn't been initialized.
			// h.stage.Logln("accessChecker: .allow and .deny conflicting rules")
			return false
		}
		return true
	}, nil)
}

func (h *accessChecker) OnPrepare() {
	h.allowRules = h.parseRule(h.allow)
	h.denyRules = h.parseRule(h.deny)

	//	sort by priority
	sort.Sort(ipRuleSlice(h.allowRules))
	sort.Sort(ipRuleSlice(h.denyRules))
}

// priority: ip > ip/24 > ip/16 > all
func (h *accessChecker) Handle(req Request, resp Response) (next bool) {
	if len(h.denyRules) == 0 {
		return true
	}
	var (
		ip        = addressToIP(req.PeerAddr().String())
		allowRank = -1
		denyRank  = -1
		allowMask []byte
		denyMask  []byte
	)

	for _, rule := range h.allowRules {
		if ip.Equal(rule.ip) {
			return true
		}
		if rule.cidr != nil && rule.rank > allowRank && rule.cidr.Contains(ip) {
			allowRank = rule.rank
			allowMask = rule.cidr.Mask
			break
		}
		if rule.all {
			allowRank = rule.rank
			break
		}
	}

	for _, rule := range h.denyRules {
		if ip.Equal(rule.ip) {
			goto forbidden
		}
		if rule.cidr != nil && rule.rank > denyRank && rule.cidr.Contains(ip) {
			denyRank = rule.rank
			denyMask = rule.cidr.Mask
			break
		}
		if rule.all {
			denyRank = rule.rank
			break
		}
	}

	if allowRank > denyRank {
		return true
	}
	if allowRank == denyRank && bytes.Compare(allowMask, denyMask) == 1 {
		return true
	}

forbidden:
	resp.SetStatus(StatusForbidden)
	resp.SendBytes(nil)
	return false
}

type ipRule struct {
	ip   net.IP
	cidr *net.IPNet
	all  bool
	rank int
}

func (r *ipRule) String() string {
	if r.all {
		return "all"
	} else if r.ip != nil {
		return r.ip.String()
	} else if r.cidr != nil {
		return r.cidr.String()
	}

	return "unkown"
}

type ipRuleSlice []*ipRule

func (r ipRuleSlice) Len() int {
	return len(r)
}
func (r ipRuleSlice) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
func (r ipRuleSlice) Less(i, j int) bool {
	if r[i].rank > r[j].rank {
		return true
	}
	if r[i].rank == r[j].rank && r[i].rank == rankCIDR &&
		bytes.Compare(r[i].cidr.Mask, r[j].cidr.Mask) == 1 {
		return true
	}
	return false
}

func checkRule(rules []string) bool {
	for _, rule := range rules {
		if rule == "all" {
			continue
		}
		if ip := net.ParseIP(rule); ip != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(rule); err == nil {
			continue
		}
		return false
	}
	return true
}

func (h *accessChecker) parseRule(rules []string) []*ipRule {
	p := make([]*ipRule, 0, len(rules))
	for _, rule := range rules {
		if rule == "all" {
			p = append(p, &ipRule{all: true, rank: rankAll})
		} else if ip := net.ParseIP(rule); ip != nil {
			p = append(p, &ipRule{ip: ip, rank: rankIP})
		} else if _, ipnet, err := net.ParseCIDR(rule); err == nil {
			p = append(p, &ipRule{cidr: ipnet, rank: rankCIDR})
		} else {
			h.stage.Logf("accessChecker illegal ip rule: %v", rule)
		}
	}
	return p
}

func checkRuleConflict(r1s, r2s []string) bool {
	for _, r1 := range r1s {
		for _, r2 := range r2s {
			if r1 == r2 {
				return true
			}
		}
	}
	return false
}

func addressToIP(address string) net.IP {
	if address[0] == '[' { // ipv6
		lastIndex := strings.IndexByte(address, ']')
		return net.ParseIP(address[1:lastIndex])
	}

	// ipv4
	lastIndex := strings.IndexByte(address, ':')
	return net.ParseIP(address[:lastIndex])
}
