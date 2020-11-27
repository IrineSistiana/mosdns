//     Copyright (C) 2020, IrineSistiana
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

package redirect_domain

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/domain"
	"github.com/miekg/dns"
)

func init() {
	handler.RegInitFunc("redirect_domain", Init)
}

type Args struct {
	Domain        []string `yaml:"domain"`
	CheckQuestion bool     `yaml:"check_question"`
	CheckCNAME    bool     `yaml:"check_cname"`
	Redirect      string   `yaml:"redirect"`
	Next          string   `yaml:"next"`
}

type checker struct {
	matcherGroup  domain.Matcher
	matchQuestion bool
	matchCNAME    bool
}

func (c *checker) Match(ctx context.Context, qCtx *handler.Context) (matched bool, err error) {
	return (c.matchQuestion && c.matchQ(qCtx)) || (c.matchCNAME && c.matchC(qCtx)), nil
}

func Init(conf *handler.Config) (p handler.Plugin, err error) {
	args := new(Args)
	err = conf.Args.WeakDecode(args)
	if err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	c := new(checker)

	// init matcher
	if len(args.Domain) == 0 {
		return nil, errors.New("no domain file")
	}

	mg := make([]domain.Matcher, 0, len(args.Domain))
	for _, f := range args.Domain {
		matcher, err := domain.NewDomainMatcherFormFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to load domain file %s: %w", f, err)
		}
		mg = append(mg, matcher)
	}

	c.matchQuestion = args.CheckQuestion
	c.matchCNAME = args.CheckCNAME
	c.matcherGroup = domain.NewMatcherGroup(mg)

	return handler.NewRedirectPlugin(conf, c, args.Next, args.Redirect), nil
}

func (c *checker) matchQ(qCtx *handler.Context) bool {
	if qCtx == nil || qCtx.Q == nil || len(qCtx.Q.Question) == 0 {
		return false
	}
	return c.matcherGroup.Match(qCtx.Q.Question[0].Name)
}

func (c *checker) matchC(qCtx *handler.Context) bool {
	if qCtx == nil || qCtx.R == nil || len(qCtx.R.Answer) == 0 {
		return false
	}
	for i := range qCtx.R.Answer {
		if cname, ok := qCtx.R.Answer[i].(*dns.CNAME); ok {
			if c.matcherGroup.Match(cname.Target) {
				return true
			}
		}
	}
	return false
}
