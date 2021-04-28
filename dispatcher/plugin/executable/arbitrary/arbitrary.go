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

package arbitrary

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	arb "github.com/IrineSistiana/mosdns/dispatcher/pkg/arbitrary"
)

const PluginType = "arbitrary"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ESExecutablePlugin = (*arbitraryPlugin)(nil)

type Args struct {
	RR []string `yaml:"rr"`
}

type arbitraryPlugin struct {
	*handler.BP
	arbitrary *arb.Arbitrary
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newArb(bp, args.(*Args))
}

func newArb(bp *handler.BP, args *Args) (*arbitraryPlugin, error) {
	a := arb.NewArbitrary()
	if err := a.BatchLoad(args.RR); err != nil {
		return nil, err
	}
	return &arbitraryPlugin{
		BP:        bp,
		arbitrary: a,
	}, nil
}

func (a *arbitraryPlugin) ExecES(_ context.Context, qCtx *handler.Context) (earlyStop bool, err error) {
	if r := a.arbitrary.LookupMsg(qCtx.Q()); r != nil {
		qCtx.SetResponse(r, handler.ContextStatusResponded)
		return true, nil
	}
	return false, nil
}
