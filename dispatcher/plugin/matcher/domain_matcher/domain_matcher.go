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

package domainmatcher

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/domain"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
)

const PluginType = "domain_matcher"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

var _ handler.Matcher = (*domainMatcher)(nil)

type Args struct {
	Domain        []string `yaml:"domain"`
	CheckQuestion bool     `yaml:"check_question"`
	CheckCNAME    bool     `yaml:"check_cname"`
}

type domainMatcher struct {
	matcher       domain.Matcher
	matchQuestion bool
	matchCNAME    bool
	logger        *logrus.Entry
}

func (c *domainMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	return (c.matchQuestion && c.matchQ(qCtx)) || (c.matchCNAME && c.matchC(qCtx)), nil
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	if len(args.Domain) == 0 {
		return nil, errors.New("no domain file is configured")
	}

	m := new(domainMatcher)
	m.logger = mlog.NewPluginLogger(tag)
	mixMatcher := domain.NewMixMatcher()
	for _, f := range args.Domain {
		err := mixMatcher.LoadFormFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to load domain file %s: %w", f, err)
		}
	}

	m.matchQuestion = args.CheckQuestion
	m.matchCNAME = args.CheckCNAME
	m.matcher = mixMatcher

	return handler.WrapMatcherPlugin(tag, PluginType, m), nil
}

func (c *domainMatcher) matchQ(qCtx *handler.Context) bool {
	if qCtx == nil || qCtx.Q == nil || len(qCtx.Q.Question) == 0 {
		return false
	}
	_, ok := c.matcher.Match(qCtx.Q.Question[0].Name)
	return ok
}

func (c *domainMatcher) matchC(qCtx *handler.Context) bool {
	if qCtx == nil || qCtx.R == nil || len(qCtx.R.Answer) == 0 {
		return false
	}
	for i := range qCtx.R.Answer {
		if cname, ok := qCtx.R.Answer[i].(*dns.CNAME); ok {
			if _, ok := c.matcher.Match(cname.Target); ok {
				return true
			}
		}
	}
	return false
}
