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

package redirect_query_type

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/rr_type"
)

const PluginType = "redirect_query_type"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

type Args struct {
	Type     []int  `yaml:"type"`
	Redirect string `yaml:"redirect"`
	Next     string `yaml:"next"`
}

type qTypeChecker struct {
	matcher *rr_type.Matcher
}

func (c *qTypeChecker) Match(ctx context.Context, qCtx *handler.Context) (matched bool, err error) {
	return c.match(qCtx), nil
}

func Init(conf *handler.Config) (p handler.Plugin, err error) {
	args := new(Args)
	err = conf.Args.WeakDecode(args)
	if err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	if len(args.Type) == 0 {
		return nil, errors.New("no type is specified")
	}

	types := make([]uint16, 0, len(args.Type))
	for _, i := range args.Type {
		types = append(types, uint16(i))
	}
	matcher, err := rr_type.NewMatcher(types)
	if err != nil {
		return nil, fmt.Errorf("invalid pattens: %w", err)
	}

	c := new(qTypeChecker)
	c.matcher = matcher
	return handler.NewRedirectPlugin(conf, c, args.Next, args.Redirect), nil
}

func (c *qTypeChecker) match(qCtx *handler.Context) bool {
	if qCtx == nil || qCtx.Q == nil || len(qCtx.Q.Question) == 0 {
		return false
	}

	return c.matcher.Match(qCtx.Q.Question[0].Qtype)
}
