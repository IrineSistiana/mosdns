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

package ipset

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const PluginType = "ipset"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*ipsetPlugin)(nil)

type Args struct {
	SetName4 string `yaml:"set_name4"`
	SetName6 string `yaml:"set_name6"`
	Mask4    uint8  `yaml:"mask4"` // default 24
	Mask6    uint8  `yaml:"mask6"` // default 32
}

type ipsetPlugin struct {
	*handler.BP
	args *Args
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newIpsetPlugin(bp, args.(*Args)), nil
}

func newIpsetPlugin(bp *handler.BP, args *Args) *ipsetPlugin {
	if args.Mask4 == 0 {
		args.Mask4 = 24
	}
	if args.Mask6 == 0 {
		args.Mask6 = 32
	}

	return &ipsetPlugin{
		BP:   bp,
		args: args,
	}
}

// Exec tries to add all qCtx.R() IPs to system ipset.
// If an error occurred, Exec will just log it.
// Therefore, Exec will never raise its own error.
func (p *ipsetPlugin) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	r := qCtx.R()
	if r != nil {
		er := p.addIPSet(r)
		if er != nil {
			p.L().Warn("failed to add response IP to ipset", qCtx.InfoField(), zap.Error(er))
		}
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}

func (p *ipsetPlugin) addIPset(r *dns.Msg) error {
	return p.addIPSet(r)
}
