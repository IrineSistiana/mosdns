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

package sleep

import (
	"context"
	"strconv"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
)

const PluginType = "sleep"

func init() {
	// Register this plugin type with its initialization funcs. So that, this plugin
	// can be configured by user from configuration file.
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })

	// You can also register a plugin object directly. (If plugin do not need to configure)
	// Then you can directly use "_sleep_500ms" in configuration file.
	coremain.RegNewPersetPluginFunc("_sleep_500ms", func(bp *coremain.BP) (any, error) {
		return &sleep{d: time.Millisecond * 500}, nil
	})

	// You can register a quick setup func for sequence. So that users can
	// init your plugin in the sequence directly in one string.
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

// Args is the arguments of plugin. It will be decoded from yaml.
// So it is recommended to use `yaml` as struct field's tag.
type Args struct {
	Duration uint `yaml:"duration"` // (milliseconds) duration for sleep.
}

var _ sequence.Executable = (*sleep)(nil)

// sleep implements handler.ExecutablePlugin.
type sleep struct {
	d time.Duration
}

// Exec implements handler.Executable.
func (s *sleep) Exec(ctx context.Context, qCtx *query_context.Context) error {
	if s.d > 0 {
		timer := pool.GetTimer(s.d)
		defer pool.ReleaseTimer(timer)
		select {
		case <-timer.C:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	return nil
}

func Init(_ *coremain.BP, args any) (any, error) {
	d := args.(*Args).Duration
	return &sleep{
		d: time.Duration(d) * time.Millisecond,
	}, nil
}

func QuickSetup(_ sequence.BQ, s string) (any, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	return &sleep{d: time.Duration(n) * time.Millisecond}, nil
}
