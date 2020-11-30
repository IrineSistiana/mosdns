//     Copyright (C) 2020, IrineSistiana
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
	"github.com/IrineSistiana/mosdns/dispatcher/logger"
)

const PluginType = "ipset"

func init() {
	handler.RegInitFunc(PluginType, Init)
	handler.SetTemArgs(PluginType, &Args{Mask4: 24, Mask6: 32})
}

var _ handler.Functional = (*ipsetPlugin)(nil)

type Args struct {
	SetName4 string `yaml:"set_name4"`
	SetName6 string `yaml:"set_name6"`
	Mask4    int    `yaml:"mask4"`
	Mask6    int    `yaml:"mask6"`
}

type ipsetPlugin struct {
	setName4, setName6 string
	mask4, mask6       uint8
}

func Init(tag string, argsMap handler.Args) (p handler.Plugin, err error) {
	args := new(Args)
	err = argsMap.WeakDecode(args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	ipsetPlugin := new(ipsetPlugin)

	ipsetPlugin.setName4 = args.SetName4
	ipsetPlugin.setName6 = args.SetName6
	ipsetPlugin.mask4 = uint8(args.Mask4)
	ipsetPlugin.mask6 = uint8(args.Mask6)

	return handler.WrapFunctionalPlugin(tag, PluginType, ipsetPlugin), nil
}

// Modify tries to add ip in qCtx.R to system ipset.
// If an error occurred, Modify will just log it. It won't stop the exec sequence.
// Therefore, Modify will never return a err.
func (p *ipsetPlugin) Do(_ context.Context, qCtx *handler.Context) (err error) {
	if qCtx == nil || qCtx.R == nil {
		return nil
	}

	er := p.addIPSet(qCtx.R)
	if er != nil {
		logger.Entry().Warn(err)
	}
	return nil
}
