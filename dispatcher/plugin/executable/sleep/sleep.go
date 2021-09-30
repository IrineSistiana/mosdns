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

package sleep

import (
	"context"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/pool"
	"time"
)

const PluginType = "sleep"

func init() {
	// Register this plugin type with its initialization funcs. So that, this plugin
	// can be configured by user from configuration file.
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	// You can also register a plugin object directly. (If plugin do not need to configure)
	// Then you can directly use "_sleep_500ms" in configuration file.
	handler.MustRegPlugin(&sleep{
		BP: handler.NewBP("_sleep_500ms", PluginType),
		d:  time.Millisecond * 500,
	}, true)
}

// Args is the arguments of plugin. It will be decoded from yaml.
// So it is recommended to use `yaml` as struct field's tag.
type Args struct {
	Duration uint `yaml:"duration"` // (milliseconds) duration for sleep.
}

var _ handler.ExecutablePlugin = (*sleep)(nil)

// sleep implements handler.ExecutablePlugin.
type sleep struct {
	*handler.BP
	d time.Duration
}

// Exec implements handler.Executable.
func (s *sleep) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	if s.d > 0 {
		timer := pool.GetTimer(s.d)
		defer pool.ReleaseTimer(timer)
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Call handler.ExecChainNode() can execute next plugin.
	return handler.ExecChainNode(ctx, qCtx, next)

	// You can control how/when to execute next plugin.
	// For more complex example, see plugin "cache".
}

// Init is a handler.NewPluginFunc.
func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	d := args.(*Args).Duration
	return &sleep{
		BP: bp,
		d:  time.Duration(d) * time.Millisecond,
	}, nil
}
