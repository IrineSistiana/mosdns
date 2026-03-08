package simple

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

import (
	"container/list"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"go.uber.org/zap"
)

const PluginType = "statistics_simple"

var _ sequence.RecursiveExecutable = (*simpleServer)(nil)

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

// Args is the arguments of plugin. It will be decoded from yaml.
// So it is recommended to use `yaml` as struct field's tag.
type Args struct {
	Size           int    `yaml:"size"`
	WebHook        string `yaml:"web_hook"`
	WebHookTimeout int    `yaml:"web_hook_timeout"`
}

func (a *Args) init() {
	utils.SetDefaultUnsignNum(&a.Size, 128)
	utils.SetDefaultUnsignNum(&a.WebHookTimeout, 5)
}

type simpleServer struct {
	args   *Args
	logger *zap.Logger

	backend *list.List
}

func Init(bp *coremain.BP, args any) (any, error) {
	ss, err := NewUiServer(args.(*Args), bp.L())
	if err != nil {
		return nil, err
	}
	bp.RegAPI(ss.Api())
	return ss, nil
}

func NewUiServer(args *Args, l *zap.Logger) (ss *simpleServer, err error) {
	args.init()

	httpClient.Timeout = time.Second * time.Duration(args.WebHookTimeout)

	logger := l
	if logger == nil {
		logger = zap.NewNop()
	}

	ss = &simpleServer{
		backend: list.New(),
		args:    args,
		logger:  logger,
	}

	return
}
