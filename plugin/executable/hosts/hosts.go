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
	"fmt"
	"os"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/hosts"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/IrineSistiana/mosdns/v5/plugin/statistics"
	"github.com/miekg/dns"
)

const PluginType = "hosts"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

var _ sequence.Executable = (*Hosts)(nil)

type Args struct {
	Entries []string `yaml:"entries"`
	Files   []string `yaml:"files"`
}

type Hosts struct {
	h *hosts.Hosts
}

func Init(_ *coremain.BP, args any) (any, error) {
	return NewHosts(args.(*Args))
}

func NewHosts(args *Args) (*Hosts, error) {
	m := domain.NewMixMatcher[*hosts.IPs]()
	m.SetDefaultMatcher(domain.MatcherFull)
	for i, entry := range args.Entries {
		if err := domain.Load[*hosts.IPs](m, entry, hosts.ParseIPs); err != nil {
			return nil, fmt.Errorf("failed to load entry #%d %s, %w", i, entry, err)
		}
	}
	for i, file := range args.Files {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file #%d %s, %w", i, file, err)
		}
		if err := domain.LoadFromTextReader[*hosts.IPs](m, bytes.NewReader(b), hosts.ParseIPs); err != nil {
			return nil, fmt.Errorf("failed to load file #%d %s, %w", i, file, err)
		}
	}

	return &Hosts{
		h: hosts.NewHosts(m),
	}, nil
}

func (h *Hosts) Response(q *dns.Msg) *dns.Msg {
	return h.h.LookupMsg(q)
}

func (h *Hosts) Exec(_ context.Context, qCtx *query_context.Context) error {
	r := h.h.LookupMsg(qCtx.Q())
	if r != nil {
		qCtx.SetResponse(r)
		qCtx.StoreValue(statistics.HostStoreKey, true)
	}
	return nil
}
