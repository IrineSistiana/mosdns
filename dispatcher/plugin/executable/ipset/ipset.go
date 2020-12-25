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
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/sirupsen/logrus"
)

const PluginType = "ipset"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

var _ handler.Executable = (*ipsetPlugin)(nil)

type Args struct {
	SetName4 string `yaml:"set_name4"`
	SetName6 string `yaml:"set_name6"`
	Mask4    uint8  `yaml:"mask4"`
	Mask6    uint8  `yaml:"mask6"`
}

type ipsetPlugin struct {
	logger *logrus.Entry
	args   *Args
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	ipsetPlugin := &ipsetPlugin{
		logger: mlog.NewPluginLogger(tag),
		args:   args,
	}

	return handler.WrapExecutablePlugin(tag, PluginType, ipsetPlugin), nil
}

// Exec tries to add all qCtx.R IPs to system ipset.
// If an error occurred, Exec will just log it.
// Therefore, Exec will never return an err.
func (p *ipsetPlugin) Exec(_ context.Context, qCtx *handler.Context) (err error) {
	if qCtx == nil || qCtx.R == nil {
		return nil
	}

	er := p.addIPSet(qCtx.R)
	if er != nil {
		p.logger.Warnf("%v: failed to add response IP to ipset: %v", qCtx, er)
	}
	return nil
}
