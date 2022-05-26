//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package redirect

import (
	"context"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/domain"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const PluginType = "redirect"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*redirectPlugin)(nil)

type Args struct {
	Rule []string `yaml:"rule"`
}

type redirectPlugin struct {
	*handler.BP
	m *domain.MixMatcher[string]
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newRedirect(bp, args.(*Args))
}

func newRedirect(bp *handler.BP, args *Args) (*redirectPlugin, error) {
	m := domain.NewMixMatcher[string]()
	m.SetDefaultMatcher(domain.MatcherFull)
	if err := domain.BatchLoad[string](m, args.Rule, func(s string) (v string, err error) {
		return dns.Fqdn(s), nil
	}); err != nil {
		return nil, err
	}
	bp.L().Info("redirect rules loaded", zap.Int("length", m.Len()))
	return &redirectPlugin{
		BP: bp,
		m:  m,
	}, nil
}

func (r *redirectPlugin) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	q := qCtx.Q()
	if len(q.Question) != 1 {
		return handler.ExecChainNode(ctx, qCtx, next)
	}
	orgQName := q.Question[0].Name
	d, ok := r.m.Match(orgQName)
	if !ok {
		return handler.ExecChainNode(ctx, qCtx, next)
	}

	q.Question[0].Name = d
	err := handler.ExecChainNode(ctx, qCtx, next)
	if r := qCtx.R(); r != nil {
		for i := range r.Question {
			if r.Question[i].Name == d {
				r.Question[i].Name = orgQName
			}
		}
		for _, a := range r.Answer {
			h := a.Header()
			if h.Name == d {
				h.Name = orgQName
			}
		}
	}
	return err
}
