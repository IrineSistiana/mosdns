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

package redirect_ip

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/netlist"
	"github.com/miekg/dns"
	"net"
)

func init() {
	handler.RegInitFunc("redirect_ip", Init)
}

type Args struct {
	IP       []string `yaml:"ip"`
	Redirect string   `yaml:"redirect"`
	Next     string   `yaml:"next"`
}

type ipChecker struct {
	matcherGroup netlist.Matcher
}

func Init(conf *handler.Config) (p handler.Plugin, err error) {
	args := new(Args)
	err = conf.Args.WeakDecode(args)
	if err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	c := new(ipChecker)

	// init matcherGroup
	if len(args.IP) == 0 {
		return nil, errors.New("no ip file")
	}

	mg := make([]netlist.Matcher, 0, len(args.IP))
	for _, f := range args.IP {
		matcher, err := netlist.NewIPMatcherFromFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to load ip file %s: %w", f, err)
		}
		mg = append(mg, matcher)
	}

	c.matcherGroup = netlist.NewMatcherGroup(mg)

	return handler.NewRedirectPlugin(conf, c, args.Next, args.Redirect), nil
}

func (c *ipChecker) Match(ctx context.Context, qCtx *handler.Context) (bool, error) {
	if qCtx == nil || qCtx.R == nil || len(qCtx.R.Answer) == 0 {
		return false, nil
	}

	for i := range qCtx.R.Answer {
		var ip net.IP
		switch rr := qCtx.R.Answer[i].(type) {
		case *dns.A:
			ip = rr.A
		case *dns.AAAA:
			ip = rr.AAAA
		default:
			continue
		}

		if c.matcherGroup.Match(ip) {
			return true, nil
		}
	}
	return false, nil
}
