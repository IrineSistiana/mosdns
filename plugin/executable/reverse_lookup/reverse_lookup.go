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

package reverselookup

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/cache"
	"github.com/IrineSistiana/mosdns/v4/pkg/cache/mem_cache"
	"github.com/IrineSistiana/mosdns/v4/pkg/cache/redis_cache"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"github.com/go-redis/redis/v8"
	"github.com/miekg/dns"
	"net"
	"net/http"
	"net/netip"
	"time"
)

const (
	PluginType = "reverse_lookup"
)

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ coremain.ExecutablePlugin = (*reverseLookup)(nil)

type Args struct {
	Size      int    `yaml:"size"` // Default is 64*1024
	Redis     string `yaml:"redis"`
	HandlePTR bool   `yaml:"handle_ptr"`
	TTL       int    `yaml:"ttl"` // Default is 1800 (30min)
}

func (a *Args) initDefault() *Args {
	if a.Size <= 0 {
		a.Size = 64 * 1024
	}
	if a.TTL <= 0 {
		a.TTL = 1800
	}
	return a
}

type reverseLookup struct {
	*coremain.BP
	args *Args
	c    cache.Backend
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newReverseLookup(bp, args.(*Args))
}

func newReverseLookup(bp *coremain.BP, args *Args) (coremain.Plugin, error) {
	args.initDefault()
	var c cache.Backend
	if u := args.Redis; len(u) > 0 {
		opts, err := redis.ParseURL(u)
		if err != nil {
			return nil, fmt.Errorf("invalid redis url, %w", err)
		}
		r := redis.NewClient(opts)
		rc, err := redis_cache.NewRedisCache(redis_cache.RedisCacheOpts{
			Client:       r,
			ClientCloser: r,
			Logger:       bp.L(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to init redis cache, %w", err)
		}
		c = rc
	} else {
		c = mem_cache.NewMemCache(args.Size, 0)
	}
	p := &reverseLookup{
		BP:   bp,
		args: args,
		c:    c,
	}
	return p, nil
}

func (p *reverseLookup) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	q := qCtx.Q()
	if r := p.handlePTRQuery(q); r != nil {
		qCtx.SetResponse(r)
		return nil
	}

	if err := executable_seq.ExecChainNode(ctx, qCtx, next); err != nil {
		return err
	}
	p.saveIPs(q, qCtx.R())
	return nil
}

func (p *reverseLookup) Close() error {
	return p.c.Close()
}

func (p *reverseLookup) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req.Context()
	ipStr := req.URL.Query().Get("ip")
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	d := p.lookup(netip.AddrFrom16(addr.As16()))
	w.Write([]byte(d))
}

func (p *reverseLookup) lookup(n netip.Addr) string {
	v, _, _ := p.c.Get(as16(n).String())
	return string(v)
}

func (p *reverseLookup) handlePTRQuery(q *dns.Msg) *dns.Msg {
	if p.args.HandlePTR && len(q.Question) > 0 && q.Question[0].Qtype == dns.TypePTR {
		question := q.Question[0]
		addr, _ := utils.ParsePTRName(question.Name)
		// If we cannot parse this ptr name. Just ignore it and pass query to next node.
		// PTR standards are a mess.
		if !addr.IsValid() {
			return nil
		}
		fqdn := p.lookup(addr)
		if len(fqdn) > 0 {
			r := new(dns.Msg)
			r.SetReply(q)
			r.Answer = append(r.Answer, &dns.PTR{
				Hdr: dns.RR_Header{
					Name:   question.Name,
					Rrtype: question.Qtype,
					Class:  question.Qclass,
					Ttl:    5,
				},
				Ptr: fqdn,
			})
			return r
		}
	}
	return nil
}

func (p *reverseLookup) saveIPs(q, r *dns.Msg) {
	if r == nil {
		return
	}

	now := time.Now()
	for _, rr := range r.Answer {
		var ip net.IP
		switch rr := rr.(type) {
		case *dns.A:
			ip = rr.A
		case *dns.AAAA:
			ip = rr.AAAA
		default:
			continue
		}

		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		h := rr.Header()
		if int(h.Ttl) > p.args.TTL {
			h.Ttl = uint32(p.args.TTL)
		}
		name := h.Name
		if len(q.Question) == 1 {
			name = q.Question[0].Name
		}
		p.c.Store(as16(addr).String(), []byte(name), now, now.Add(time.Duration(p.args.TTL)*time.Second))
	}
}

func as16(n netip.Addr) netip.Addr {
	if n.Is6() {
		return n
	}
	return netip.AddrFrom16(n.As16())
}
