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

package qtypematcher

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/rr_type"
	"github.com/miekg/dns"
)

const PluginType = "qtype_matcher"

func init() {
	handler.RegInitFunc(PluginType, Init)
	handler.SetTemArgs(PluginType, &Args{Type: []uint16{1, 28}})

	handler.MustRegPlugin(handler.WrapMatcherPlugin("_qtype_A_AAAA", PluginType,
		&qTypeMatcher{matcher: rr_type.NewMatcher([]uint16{dns.TypeA, dns.TypeAAAA})}))
}

var _ handler.Matcher = (*qTypeMatcher)(nil)

type Args struct {
	Type []uint16 `yaml:"type"`
}

type qTypeMatcher struct {
	matcher *rr_type.Matcher
}

func (c *qTypeMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	return c.match(qCtx), nil
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	if len(args.Type) == 0 {
		return nil, errors.New("no type is specified")
	}

	c := new(qTypeMatcher)
	c.matcher = rr_type.NewMatcher(args.Type)
	return handler.WrapMatcherPlugin(tag, PluginType, c), nil
}

func (c *qTypeMatcher) match(qCtx *handler.Context) bool {
	if qCtx == nil || qCtx.Q == nil || len(qCtx.Q.Question) == 0 {
		return false
	}

	return c.matcher.Match(qCtx.Q.Question[0].Qtype)
}
