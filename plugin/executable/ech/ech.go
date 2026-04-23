/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package ech

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider/ip_set"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/cache"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/forward"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"
)

const PluginType = "ech"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

type Match struct {
	IpSetTag     string `yaml:"ip_set"`
	CacheTag     string `yaml:"cache"`
	PollInterval uint   `yaml:"poll_interval"`
	MaxDelay     uint   `yaml:"max_delay"`
}

type Args struct {
	ECH        string `yaml:"ech"`
	ForwardTag string `yaml:"forward"`
	ECHQName   string `yaml:"qname"`

	Match *Match `yaml:"match"`
}

type match struct {
	matcher      netlist.Matcher
	cache        *cache.Cache
	pollInterval time.Duration
	maxDelay     time.Duration
}

type tags struct {
	forward string
	ipset   string
	cache   string
}

type impl struct {
	ech     []byte
	forward *fastforward.Forward
	qname   string

	match *match
	tags  tags
}

func fromArgs(args *Args, cm *coremain.Mosdns, logger *zap.Logger) (*impl, error) {
	if args.ECH == "" && args.ForwardTag == "" {
		return nil, fmt.Errorf("ech config: should configure either `ech` or `forward`")
	}

	var ech []byte
	if args.ECH != "" {
		bs, err := base64.StdEncoding.DecodeString(args.ECH)
		if err != nil {
			return nil, err
		}
		err = validateECH(bs)
		if err != nil {
			return nil, err
		}
		ech = bs
	}

	m := args.Match
	logger.Info(("ech init, configuration"),
		zap.String("ech", args.ECH),
		zap.String("forward_tag", args.ForwardTag),
		zap.String("ech_qname", args.ECHQName))

	if m != nil {
		if m.IpSetTag == "" || m.CacheTag == "" {
			return nil, fmt.Errorf("ech config: match should have ip_set and cache")
		}
		m.PollInterval = max(m.PollInterval, 100)
		if m.MaxDelay == 0 {
			m.MaxDelay = 5000
		}
		m.MaxDelay = max(m.MaxDelay, 500)
		logger.Info(("ech init, configuration"),
			zap.String("match.ip_set", args.Match.IpSetTag),
			zap.String("match.cache", args.Match.CacheTag),
			zap.Uint("match.poll_interval", args.Match.PollInterval),
			zap.Uint("match.max_delay", args.Match.MaxDelay))
	}

	fw, is, cc, err := getPlugins(cm, args)
	if err != nil {
		return nil, err
	}

	var im *impl
	if m == nil {
		im = &impl{ech, fw, args.ECHQName, nil, tags{forward: args.ForwardTag}}
	} else {
		im = &impl{ech, fw, args.ECHQName, &match{is.GetIPMatcher(), cc, time.Duration(args.Match.PollInterval) * time.Millisecond, time.Duration(args.Match.MaxDelay) * time.Millisecond}, tags{args.ForwardTag, args.Match.IpSetTag, args.Match.CacheTag}}
	}
	return im, nil
}

type rrLocal struct {
	ttl uint32
	ts  int64

	rr  *dns.HTTPS
	ech *dns.SVCBECHConfig
}

type ECH struct {
	*impl
	l *zap.Logger

	rr atomic.Pointer[rrLocal]
}

func getPlugins(cm *coremain.Mosdns, args *Args) (*fastforward.Forward, *ip_set.IPSet, *cache.Cache, error) {
	var fw *fastforward.Forward
	var is *ip_set.IPSet
	var cc *cache.Cache
	var ok bool

	if args.ECH == "" {
		fw, ok = cm.GetPlugin(args.ForwardTag).(*fastforward.Forward)
		if !ok {
			return nil, nil, nil, fmt.Errorf("no forward with tag `%s`", args.ForwardTag)
		}
	}

	if m := args.Match; m != nil {
		is, ok = cm.GetPlugin(m.IpSetTag).(*ip_set.IPSet)
		if !ok {
			return nil, nil, nil, fmt.Errorf("no ip_set with tag `%s`", m.IpSetTag)
		}
		cc, ok = cm.GetPlugin(m.CacheTag).(*cache.Cache)
		if !ok {
			return nil, nil, nil, fmt.Errorf("no cache with tag `%s`", m.CacheTag)
		}
	}

	return fw, is, cc, nil
}

func Init(bp *coremain.BP, args any) (any, error) {
	impl, err := fromArgs(args.(*Args), bp.M(), bp.L())
	if err != nil {
		return nil, err
	}
	return newECH(impl, bp.L()), nil
}

func QuickSetup(bq sequence.BQ, args string) (any, error) {
	fs := strings.Fields(args)
	var ech, fwt, qn string
	switch len(fs) {
	case 1:
		ech = fs[0]
	case 2:
		fwt, qn = fs[0], fs[1]
	default:
		return nil, fmt.Errorf("wrong parameters, usage: ech [ $forward server_name | base64-string ]")
	}

	impl, err := fromArgs(&Args{ech, fwt, qn, nil}, bq.M(), bq.L())
	if err != nil {
		return nil, err
	}

	return newECH(impl, bq.L()), nil
}

func newECH(impl *impl, logger *zap.Logger) *ECH {
	impl.qname = dns.Fqdn(impl.qname)

	n := &ECH{impl: impl, l: logger}
	if impl.ech != nil {
		ech := dns.SVCBECHConfig{impl.ech}
		ht := dns.HTTPS{
			dns.SVCB{dns.RR_Header{".", dns.TypeHTTPS, dns.ClassINET, 0, 0},
				1, ".", nil}}
		ht.Value = append(ht.Value, &ech)
		n.rr.Store(&rrLocal{0, 0, &ht, &ech})
	} else {
		go n.backgroundResolve()
	}
	return n
}

func (p *ECH) backgroundResolve() {
	const (
		FailureRetryInterval = 5 * time.Second
		MinimumRetryInterval = 200 * time.Millisecond
	)
	var nextResolve time.Duration

	for {
		select {
		case s := <-time.After(nextResolve):
			ht, ech, ttl, err := resolveECH(p.impl.forward, p.impl.qname)
			if err != nil {
				nextResolve = FailureRetryInterval
				p.l.Warn(fmt.Sprintf("ech : resolve ECH failed, %v", err),
					zap.String("forward_tag", p.impl.tags.forward),
					zap.String("ech_qname", p.impl.qname),
					zap.Duration("next_resolve", nextResolve))
				continue
			}

			p.rr.Store(&rrLocal{ttl, time.Now().Unix(), ht, ech})

			roundtrip := time.Since(s)
			effectiveTTL := max(time.Duration(ttl)*time.Second-roundtrip/2, 0)
			nextResolve = max(effectiveTTL-3*roundtrip/2, MinimumRetryInterval)

			p.l.Info("ech : update ECHConfigList",
				zap.String("forward_tag", p.impl.tags.forward),
				zap.String("ech_qname", p.impl.qname),
				zap.String("ech", base64.StdEncoding.EncodeToString(ech.ECH)),
				zap.Uint32("ttl", ttl),
				zap.Duration("forward_roundtrip", roundtrip),
				zap.Duration("next_resolve", nextResolve))
		}
	}
}

// ///////////////////////////////////////////////////////////////////
// Non-trival clients would send HTTPS/A/AAAA query simutaneously,
// we can piggyback on their query and just poll for response.
// ///////////////////////////////////////////////////////////////////
func (p *ECH) pollMatchCache(qname string) bool {
	tl := time.Now().Add(p.impl.match.maxDelay)

	for {
		p.l.Debug("ech poll match cache",
			zap.String("forward_tag", p.impl.tags.forward),
			zap.String("ech_qname", p.impl.qname),
			zap.String("qname", qname))
		ips := p.impl.match.cache.GetIPByQName(qname)
		for i, ip := range ips {
			p.l.Debug("ech match ip ",
				zap.String("forward_tag", p.impl.tags.forward),
				zap.String("ech_qname", p.impl.qname),
				zap.Int("i", i),
				zap.String("ip", ip.String()))
			if addr, ok := netip.AddrFromSlice(ip); ok {
				if p.impl.match.matcher.Match(addr) {
					return true
				}
			}
		}

		// assume A and AAAA have same ECHConfig
		if ips != nil {
			p.l.Debug("ech poll match cache NOT match",
				zap.String("forward_tag", p.impl.tags.forward),
				zap.String("ech_qname", p.impl.qname),
				zap.String("qname", qname))
			return false
		}

		if time.Now().After(tl) {
			break
		}
		p.l.Debug(fmt.Sprintf("ech poll match next poll in %s", p.impl.match.pollInterval),
			zap.String("forward_tag", p.impl.tags.forward),
			zap.String("ech_qname", p.impl.qname),
			zap.String("qname", qname))
		time.Sleep(p.impl.match.pollInterval)
	}

	return false
}

func (p *ECH) Exec(_ context.Context, qCtx *query_context.Context) error {
	q := qCtx.QQuestion()
	if q.Qtype != dns.TypeHTTPS {
		return nil
	}

	qname := q.Name
	r := qCtx.R()

	if p.impl.match != nil && !p.pollMatchCache(qCtx.QQuestion().Name) {
		p.l.Debug("ech poll match cache NOT match",
			zap.String("forward_tag", p.impl.tags.forward),
			zap.String("ech_qname", p.impl.qname),
			zap.String("qname", qname))
		if r == nil {
			emptyResponse(qCtx)
		}
		return nil
	}

	rr := p.rr.Load()
	if rr == nil {
		p.l.Warn("ech failed: empty ECHConfigList, configuration or network error",
			zap.String("forward_tag", p.impl.tags.forward),
			zap.String("ech_qname", p.impl.qname),
			zap.String("qname", qname))
		return fmt.Errorf("ech failed: empty ECHConfigList, configuration or network error")
	}

	if r == nil {
		return p.directResponse(rr, qname, qCtx)
	} else {
		return p.filterResponse(rr, r, qname)
	}
}
