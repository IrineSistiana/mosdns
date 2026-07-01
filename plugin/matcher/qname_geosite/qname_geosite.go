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

package qname_geosite

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
)

const PluginType = "qname_geosite"

func init() {
	sequence.MustRegMatchQuickSetup(PluginType, QuickSetup)
}

func QuickSetup(bq sequence.BQ, s string) (sequence.Matcher, error) {
	if len(s) == 0 {
		return nil, errors.New("a geosite plugin and code are required")
	}

	args := strings.Fields(s)
	if len(args) != 2 {
		return nil, errors.New("args error, must like: qname_geosite $plugin_name cn")
	}

	geositeName, _ := cutPrefix(args[0], "$")

	p := bq.M().GetPlugin(geositeName)
	provider, _ := p.(data_provider.GeositeMatcherProvider)
	if provider == nil {
		return nil, fmt.Errorf("cannot find geosite %s", geositeName)
	}
	m, ok := provider.GetGeositeMatcher(args[1])
	if !ok {
		return nil, fmt.Errorf("cannot find geosite code %s", args[1])
	}

	return &Matcher{m: m}, nil
}

type Matcher struct {
	m domain.Matcher[struct{}]
}

func (m *Matcher) Match(_ context.Context, qCtx *query_context.Context) (bool, error) {
	for _, question := range qCtx.Q().Question {
		if _, ok := m.m.Match(question.Name); ok {
			return true, nil
		}
	}
	return false, nil
}

func cutPrefix(s string, p string) (string, bool) {
	if strings.HasPrefix(s, p) {
		return strings.TrimPrefix(s, p), true
	}
	return s, false
}
