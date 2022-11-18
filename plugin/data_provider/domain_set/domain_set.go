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
	Exps  []string   `yaml:"exps"`
	Sets  []string   `yaml:"sets"`
	Files []FileArgs `yaml:"files"`
}

type FileArgs struct {
	Path string `yaml:"path"`
	Type string `yaml:"type"`
	Args string `yaml:"args"`
}

type DomainSetProvider interface {
	GetDomainSet() domain.Matcher[struct{}]
}

var _ DomainSetProvider = (*DomainSet)(nil)

type DomainSet struct {
	*coremain.BP

	mg []domain.Matcher[struct{}]
}

func (d *DomainSet) GetDomainSet() domain.Matcher[struct{}] {
	return matcherGroup(d.mg)
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
		provider, _ := bp.M().GetPlugins(tag).(DomainSetProvider)
		if provider == nil {
			return nil, fmt.Errorf("%s is not a DomainSetProvider", tag)
		}
		m := provider.GetDomainSet()
		ds.mg = append(ds.mg, m)
	}
	bp.L().Info("domain set loaded", zap.Int("length", matcherGroup(ds.mg).Len()))
	return ds, nil
}

func LoadExpsAndFiles(exps []string, fs []FileArgs, m *domain.MixMatcher[struct{}]) error {
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

func LoadFiles(args []FileArgs, m *domain.MixMatcher[struct{}]) error {
	for i, f := range args {
		if err := LoadFile(f, m); err != nil {
			return fmt.Errorf("failed to load file #%d %s, %w", i, f.Path, err)
		}
	}
	return nil
}

func LoadFile(r FileArgs, m *domain.MixMatcher[struct{}]) error {
	if len(r.Path) > 0 {
		b, err := os.ReadFile(r.Path)
		if err != nil {
			return err
		}
		switch r.Type {
		case "", "list":
			if err := domain.LoadFromTextReader[struct{}](m, bytes.NewReader(b), nil); err != nil {
				return err
			}
		case "geosite":
			v, err := domain.LoadGeoSiteList(b)
			if err != nil {
				return err
			}
			pickers := domain.ParseV2Suffix(r.Args)
			return domain.LoadFromGeoSite(m, v, pickers...)
		}
	}
	return nil
}
