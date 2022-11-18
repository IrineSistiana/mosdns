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

package ip_set

import (
	"bytes"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/netlist"
	"go.uber.org/zap"
	"net/netip"
	"os"
	"strings"
)

const PluginType = "ip_set"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

func Init(bp *coremain.BP, args interface{}) (coremain.Plugin, error) {
	return NewIPSet(bp, args.(*Args))
}

type Args struct {
	IPs   []string   `yaml:"ips"`
	Sets  []string   `yaml:"sets"`
	Files []FileArgs `yaml:"files"`
}

type FileArgs struct {
	Path string `yaml:"path"`
	Type string `yaml:"type"`
	Args string `yaml:"args"`
}

type IPSetProvider interface {
	GetIPSet() netlist.Matcher
}

var _ IPSetProvider = (*IPSet)(nil)

type IPSet struct {
	*coremain.BP

	mg []netlist.Matcher
}

func (d *IPSet) GetIPSet() netlist.Matcher {
	return matcherGroup(d.mg)
}

func NewIPSet(bp *coremain.BP, args *Args) (*IPSet, error) {
	p := &IPSet{BP: bp}

	l := netlist.NewList()
	if err := LoadFromIPsAndFiles(args.IPs, args.Files, l); err != nil {
		return nil, err
	}
	l.Sort()
	if l.Len() > 0 {
		p.mg = append(p.mg, l)
	}
	for _, tag := range args.Sets {
		provider, _ := bp.M().GetPlugins(tag).(IPSetProvider)
		if provider == nil {
			return nil, fmt.Errorf("%s is not an IPSetProvider", tag)
		}
		p.mg = append(p.mg, provider.GetIPSet())
	}
	bp.L().Info("ip set loaded", zap.Int("length", matcherGroup(p.mg).Len()))
	return p, nil
}

func parseNetipPrefix(s string) (netip.Prefix, error) {
	if strings.ContainsRune(s, '/') {
		return netip.ParsePrefix(s)
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return addr.Prefix(addr.BitLen())
}

func LoadFromIPsAndFiles(ips []string, fs []FileArgs, l *netlist.List) error {
	if err := LoadFromIPs(ips, l); err != nil {
		return err
	}
	if err := LoadFromFiles(fs, l); err != nil {
		return err
	}
	return nil
}

func LoadFromIPs(ips []string, l *netlist.List) error {
	for i, s := range ips {
		p, err := parseNetipPrefix(s)
		if err != nil {
			return fmt.Errorf("invalid ip #%d %s, %w", i, s, err)
		}
		l.Append(p)
	}
	return nil
}

func LoadFromFiles(fs []FileArgs, l *netlist.List) error {
	for i, f := range fs {
		if err := LoadFromFile(f, l); err != nil {
			return fmt.Errorf("failed to load file #%d %s, %w", i, f.Path, err)
		}
	}
	return nil
}

func LoadFromFile(f FileArgs, l *netlist.List) error {
	if len(f.Path) > 0 {
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return err
		}
		switch f.Type {
		case "", "list":
			if err := netlist.LoadFromReader(l, bytes.NewReader(b)); err != nil {
				return err
			}
		case "geoip":
			v, err := netlist.LoadGeoIPListFromDAT(b)
			if err != nil {
				return err
			}
			if err := netlist.LoadIPDat(l, v, f.Args); err != nil {
				return err
			}
		}
	}
	return nil
}

type matcherGroup []netlist.Matcher

func (mg matcherGroup) Match(addr netip.Addr) bool {
	for _, m := range mg {
		if m.Match(addr) {
			return true
		}
	}
	return false
}

func (mg matcherGroup) Len() int {
	s := 0
	for _, m := range mg {
		s += m.Len()
	}
	return s
}
