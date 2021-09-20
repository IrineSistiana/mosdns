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

package strip

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"strings"
)

const PluginType = "strip"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*strip)(nil)

type Args struct {
	Types []string `yaml:"types"` // types of records to strip
}

type strip struct {
	*handler.BP
	types map[string]bool
}

func (s *strip) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	if qCtx.R() == nil {
		return nil
	}
	r := qCtx.R()
	var stripped []dns.RR
	for i := range r.Answer {
		var t = fmt.Sprintf("%T", r.Answer[i])[5:]
		if s.types[t] {
			s.L().Debug("removing " + r.Answer[i].String())
		} else {
			stripped = append(stripped, r.Answer[i])
		}
	}
	r.Answer = stripped
	return nil
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newStripPlugin(bp, args.(*Args))
}

func newStripPlugin(bp *handler.BP, args *Args) (p handler.Plugin, err error) {
	if len(args.Types) == 0 {
		return nil, fmt.Errorf("record type(s) to be removed not specified")
	}
	sp := new(strip)
	sp.BP = bp
	sp.types = make(map[string]bool)
	for _, t := range(args.Types) {
		if st := strings.ReplaceAll(t, " ", ""); st != "" {
			sp.types[st] = true
			bp.L().Debug("will be stripping " + st)
		}
	}
	return sp, nil
}
