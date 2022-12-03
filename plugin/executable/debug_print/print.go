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

package debug_print

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"go.uber.org/zap"
)

const PluginType = "debug_print"

func init() {
	sequence.MustRegQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Executable = (*debugPrint)(nil)

type debugPrint struct {
	sequence.BQ
	msg string
}

// QuickSetup format: s is the log message string. Default is "debug print".
func QuickSetup(bq sequence.BQ, s string) (any, error) {
	if len(s) == 0 {
		s = "debug print"
	}
	return &debugPrint{BQ: bq, msg: s}, nil
}

func (b *debugPrint) Exec(_ context.Context, qCtx *query_context.Context) error {
	b.BQ.L().Info(b.msg, zap.Stringer("query", qCtx.Q()))
	if r := qCtx.R(); r != nil {
		b.BQ.L().Info(b.msg, zap.Stringer("response", r))
	}
	return nil
}
