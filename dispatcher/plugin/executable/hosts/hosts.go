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

package hosts

import (
	"context"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/hosts"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/domain"
)

const PluginType = "hosts"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*hostsPlugin)(nil)

type Args struct {
	Hosts []string `yaml:"hosts"`
}

type hostsPlugin struct {
	*handler.BP
	h *hosts.Hosts
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newHostsContainer(bp, args.(*Args))
}

func newHostsContainer(bp *handler.BP, args *Args) (*hostsPlugin, error) {
	mixMatcher := domain.NewMixMatcher[*hosts.IPs]()
	mixMatcher.SetDefaultMatcher(domain.MatcherFull)
	err := domain.BatchLoad[*hosts.IPs](mixMatcher, args.Hosts, hosts.ParseIPs)
	if err != nil {
		return nil, err
	}
	return &hostsPlugin{
		BP: bp,
		h:  hosts.NewHosts(mixMatcher),
	}, nil
}

func (h *hostsPlugin) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	r := h.h.LookupMsg(qCtx.Q())
	if r != nil {
		qCtx.SetResponse(r, handler.ContextStatusResponded)
		return nil
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}
