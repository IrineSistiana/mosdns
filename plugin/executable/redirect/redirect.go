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

package redirect

import (
	"bytes"
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"strings"
)

const PluginType = "redirect"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ coremain.ExecutablePlugin = (*redirectPlugin)(nil)

type Args struct {
	Rule []string `yaml:"rule"`
}

type redirectPlugin struct {
	*coremain.BP
	m domain.Matcher[string]
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newRedirect(bp, args.(*Args))
}

func newRedirect(bp *coremain.BP, args *Args) (*redirectPlugin, error) {
	parseFunc := func(s string) (p, v string, err error) {
		f := strings.Fields(s)
		if len(f) != 2 {
			return "", "", fmt.Errorf("redirect rule must have 2 fields, but got %d", len(f))
		}
		return f[0], dns.Fqdn(f[1]), nil
	}
	staticMatcher := domain.NewMixMatcher[string]()
	staticMatcher.SetDefaultMatcher(domain.MatcherFull)
	m, err := domain.BatchLoadProvider[string](
		args.Rule,
		staticMatcher,
		parseFunc,
		bp.M().GetDataManager(),
		func(b []byte) (domain.Matcher[string], error) {
			mixMatcher := domain.NewMixMatcher[string]()
			mixMatcher.SetDefaultMatcher(domain.MatcherFull)
			if err := domain.LoadFromTextReader[string](mixMatcher, bytes.NewReader(b), parseFunc); err != nil {
				return nil, err
			}
			return mixMatcher, nil
		},
	)
	if err != nil {
		return nil, err
	}
	bp.L().Info("redirect rules loaded", zap.Int("length", m.Len()))
	return &redirectPlugin{
		BP: bp,
		m:  m,
	}, nil
}

func (r *redirectPlugin) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	q := qCtx.Q()
	if len(q.Question) != 1 || q.Question[0].Qclass != dns.ClassINET {
		return executable_seq.ExecChainNode(ctx, qCtx, next)
	}

	orgQName := q.Question[0].Name
	redirectTarget, ok := r.m.Match(orgQName)
	if !ok {
		return executable_seq.ExecChainNode(ctx, qCtx, next)
	}

	q.Question[0].Name = redirectTarget
	err := executable_seq.ExecChainNode(ctx, qCtx, next)
	if r := qCtx.R(); r != nil {
		// Restore original query name.
		for i := range r.Question {
			if r.Question[i].Name == redirectTarget {
				r.Question[i].Name = orgQName
			}
		}

		// Insert a CNAME record.
		newAns := make([]dns.RR, 1, len(r.Answer)+1)
		newAns[0] = &dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   orgQName,
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
				Ttl:    1,
			},
			Target: redirectTarget,
		}
		newAns = append(newAns, r.Answer...)
		r.Answer = newAns
	}
	return err
}
