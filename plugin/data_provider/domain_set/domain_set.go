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

package domain_set

import (
	"bytes"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider"
	"go.uber.org/zap"
	"os"
)

const PluginType = "domain_set"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

func Init(bp *coremain.BP, args interface{}) (coremain.Plugin, error) {
	return NewDomainSet(bp, args.(*Args))
}

type Args struct {
	Exps  []string `yaml:"exps"`
	Sets  []string `yaml:"sets"`
	Files []string `yaml:"files"`
}

var _ data_provider.DomainMatcherProvider = (*DomainSet)(nil)

type DomainSet struct {
	*coremain.BP

	mg []domain.Matcher[struct{}]
}

func (d *DomainSet) GetDomainMatcher() domain.Matcher[struct{}] {
	return MatcherGroup(d.mg)
}

func NewDomainSet(bp *coremain.BP, args *Args) (*DomainSet, error) {
	ds := &DomainSet{BP: bp}

	m := domain.NewDomainMixMatcher()
	if err := LoadExpsAndFiles(args.Exps, args.Files, m); err != nil {
		return nil, err
	}
	if m.Len() > 0 {
		ds.mg = append(ds.mg, m)
	}

	for _, tag := range args.Sets {
		provider, _ := bp.M().GetPlugins(tag).(data_provider.DomainMatcherProvider)
		if provider == nil {
			return nil, fmt.Errorf("%s is not a DomainMatcherProvider", tag)
		}
		m := provider.GetDomainMatcher()
		ds.mg = append(ds.mg, m)
	}
	bp.L().Info("domain set loaded", zap.Int("length", MatcherGroup(ds.mg).Len()))
	return ds, nil
}

func LoadExpsAndFiles(exps []string, fs []string, m *domain.MixMatcher[struct{}]) error {
	if err := LoadExps(exps, m); err != nil {
		return err
	}
	if err := LoadFiles(fs, m); err != nil {
		return err
	}
	return nil
}

func LoadExps(exps []string, m *domain.MixMatcher[struct{}]) error {
	for i, exp := range exps {
		if err := m.Add(exp, struct{}{}); err != nil {
			return fmt.Errorf("failed to load expression #%d %s, %w", i, exp, err)
		}
	}
	return nil
}

func LoadFiles(fs []string, m *domain.MixMatcher[struct{}]) error {
	for i, f := range fs {
		if err := LoadFile(f, m); err != nil {
			return fmt.Errorf("failed to load file #%d %s, %w", i, f, err)
		}
	}
	return nil
}

func LoadFile(f string, m *domain.MixMatcher[struct{}]) error {
	if len(f) > 0 {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}

		if err := domain.LoadFromTextReader[struct{}](m, bytes.NewReader(b), nil); err != nil {
			return err
		}
	}
	return nil
}
