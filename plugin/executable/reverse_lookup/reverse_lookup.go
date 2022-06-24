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
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
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
	TTL int // Default is 10.
}

type reverseLookup struct {
	*coremain.BP
	args  *Args
	store *store
}

func (p *reverseLookup) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ipStr := req.URL.Query().Get("ip")
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	d := p.store.lookup(addr)
	w.Write([]byte(d))
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newReverseLookup(bp, args.(*Args)), nil
}

func newReverseLookup(bp *coremain.BP, args *Args) coremain.Plugin {
	p := &reverseLookup{
		BP:    bp,
		args:  args,
		store: newStore(),
	}
	return p
}

func (p *reverseLookup) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	if err := executable_seq.ExecChainNode(ctx, qCtx, next); err != nil {
		return err
	}
	r := qCtx.R()
	if r == nil {
		return nil
	}

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
			return fmt.Errorf("invalid ip %s", ip)
		}

		h := rr.Header()
		ttl := uint32(p.args.TTL)
		if ttl == 0 {
			ttl = 10
		}
		if h.Ttl > ttl {
			h.Ttl = ttl
		}
		p.store.save(rr.Header().Name, time.Duration(ttl)*time.Second, addr)
	}
	return nil
}

func (p *reverseLookup) Close() error {
	p.store.close()
	return nil
}
