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

package hosts

import (
	"bytes"
	"context"
	"github.com/sieveLau/mosdns/v4-maintenance/coremain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/executable_seq"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/hosts"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/matcher/domain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
	"io"
)

const PluginType = "hosts"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ coremain.ExecutablePlugin = (*hostsPlugin)(nil)

type Args struct {
	Hosts []string `yaml:"hosts"`
}

type hostsPlugin struct {
	*coremain.BP
	h             *hosts.Hosts
	matcherCloser io.Closer
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newHostsContainer(bp, args.(*Args))
}

func newHostsContainer(bp *coremain.BP, args *Args) (*hostsPlugin, error) {
	staticMatcher := domain.NewMixMatcher[*hosts.IPs]()
	staticMatcher.SetDefaultMatcher(domain.MatcherFull)
	m, err := domain.BatchLoadProvider[*hosts.IPs](
		args.Hosts,
		staticMatcher,
		hosts.ParseIPs,
		bp.M().GetDataManager(),
		func(b []byte) (domain.Matcher[*hosts.IPs], error) {
			mixMatcher := domain.NewMixMatcher[*hosts.IPs]()
			mixMatcher.SetDefaultMatcher(domain.MatcherFull)
			if err := domain.LoadFromTextReader[*hosts.IPs](mixMatcher, bytes.NewReader(b), hosts.ParseIPs); err != nil {
				return nil, err
			}
			return mixMatcher, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &hostsPlugin{
		BP:            bp,
		h:             hosts.NewHosts(m),
		matcherCloser: m,
	}, nil
}

func (h *hostsPlugin) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	r := h.h.LookupMsg(qCtx.Q())
	if r != nil {
		qCtx.SetResponse(r)
		return nil
	}

	return executable_seq.ExecChainNode(ctx, qCtx, next)
}

func (h *hostsPlugin) Close() error {
	_ = h.matcherCloser.Close()
	return nil
}
