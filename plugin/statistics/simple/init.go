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

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"go.uber.org/zap"
)

const PluginType = "ui_simple"

var _ sequence.RecursiveExecutable = (*UiServer)(nil)

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
	utils.SetDefaultUnsignNum(&a.Size, 5)
}

type UiServer struct {
	args   *Args
	logger *zap.Logger

	backend *list.List
}

func Init(bp *coremain.BP, args any) (any, error) {
	c := NewUiServer(args.(*Args), bp.L())
	bp.RegAPI(c.Api())
	return c, nil
}

func NewUiServer(args *Args, l *zap.Logger) *UiServer {
	args.init()

	logger := l
	if logger == nil {
		logger = zap.NewNop()
	}

	p := &UiServer{
		backend: list.New(),
		args:    args,
		logger:  logger,
	}

	return p
}
