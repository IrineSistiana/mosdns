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

package misc_optm

import (
	"context"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"math/rand"
)

const (
	PluginType = "misc_optm"
)

func init() {
	coremain.RegNewPersetPluginFunc("_misc_optm", func(bp *coremain.BP) (coremain.Plugin, error) {
		return &optm{BP: bp}, nil
	})
}

var _ coremain.ExecutablePlugin = (*optm)(nil)

type optm struct {
	*coremain.BP
}

func (t *optm) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	q := qCtx.Q()

	// Block query that is unusual.
	if isUnusualQuery(q) {
		r := new(dns.Msg)
		r.SetRcode(q, dns.RcodeRefused)
		qCtx.SetResponse(r)
		return nil
	}
	if err := executable_seq.ExecChainNode(ctx, qCtx, next); err != nil {
		return err
	}

	r := qCtx.R()
	if r == nil {
		return nil
	}

	// Trim and shuffle answers for A and AAAA.
	switch qt := q.Question[0].Qtype; qt {
	case dns.TypeA, dns.TypeAAAA:
		rr := r.Answer[:0]
		for _, ar := range r.Answer {
			if ar.Header().Rrtype == qt {
				rr = append(rr, ar)
			}
			ar.Header().Name = q.Question[0].Name
		}

		rand.Shuffle(len(rr), func(i, j int) {
			rr[i], rr[j] = rr[j], rr[i]
		})

		r.Answer = rr
	}

	// Remove padding
	if rOpt := r.IsEdns0(); rOpt != nil {
		dnsutils.RemoveEDNS0Option(rOpt, dns.EDNS0PADDING)
	}

	// EDNS0 consistency
	if qOpt := q.IsEdns0(); qOpt == nil {
		dnsutils.RemoveEDNS0(r)
	}
	return nil
}

func isUnusualQuery(q *dns.Msg) bool {
	return !isValidQuery(q) || len(q.Question) != 1 || q.Question[0].Qclass != dns.ClassINET
}

func isValidQuery(q *dns.Msg) bool {
	return !q.Response && q.Opcode == dns.OpcodeQuery && !q.Authoritative && !q.Zero && // check header
		len(q.Answer) == 0 && len(q.Ns) == 0 // check body
}
