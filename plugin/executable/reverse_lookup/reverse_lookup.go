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
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/cache"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/go-chi/chi/v5"
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
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

var _ sequence.RecursiveExecutable = (*ReverseLookup)(nil)

type Args struct {
	Size      int  `yaml:"size"` // Default is 64*1024
	HandlePTR bool `yaml:"handle_ptr"`
	TTL       int  `yaml:"ttl"` // Default is 7200 (2h)
}

func (a *Args) init() {
	utils.SetDefaultUnsignNum(&a.Size, 64*1024)
	utils.SetDefaultUnsignNum(&a.TTL, 7200)
}

type ReverseLookup struct {
	args *Args
	c    *cache.Cache[key, string]
}

func Init(bp *coremain.BP, args any) (any, error) {
	return NewReverseLookup(bp, args.(*Args))
}

func NewReverseLookup(bp *coremain.BP, args *Args) (any, error) {
	args.init()
	c := cache.New[key, string](cache.Opts{Size: args.Size})
	p := &ReverseLookup{
		args: args,
		c:    c,
	}
	r := chi.NewRouter()
	r.Get("/", p.ServeHTTP)
	bp.RegAPI(r)
	return p, nil
}

func (p *ReverseLookup) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	q := qCtx.Q()
	if r := p.ResponsePTR(q); r != nil {
		qCtx.SetResponse(r)
		return nil
	}

	if err := next.ExecNext(ctx, qCtx); err != nil {
		return err
	}
	p.saveIPs(q, qCtx.R())
	return nil
}

func (p *ReverseLookup) Close() error {
	return p.c.Close()
}

func (p *ReverseLookup) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ipStr := req.URL.Query().Get("ip")
	if len(ipStr) == 0 {
		http.Error(w, "no 'ip' query parameter found", http.StatusBadRequest)
		return
	}
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d := p.lookup(netip.AddrFrom16(addr.As16()))
	if len(d) > 0 {
		_, _ = fmt.Fprint(w, d)
	}
}

func (p *ReverseLookup) lookup(n netip.Addr) string {
	v, _, _ := p.c.Get(key(as16(n)))
	return v
}

func (p *ReverseLookup) ResponsePTR(q *dns.Msg) *dns.Msg {
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

func (p *ReverseLookup) saveIPs(q, r *dns.Msg) {
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
		p.c.Store(key(as16(addr)), name, now.Add(time.Duration(p.args.TTL)*time.Second))
	}
}

func as16(n netip.Addr) netip.Addr {
	if n.Is6() {
		return n
	}
	return netip.AddrFrom16(n.As16())
}
