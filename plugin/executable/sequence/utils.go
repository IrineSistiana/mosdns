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

package sequence

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
)

// reWrapper converts RecursiveExecutable to Executable
type reWrapper struct {
	re RecursiveExecutable
}

func (r *reWrapper) Exec(ctx context.Context, qCtx *query_context.Context) error {
	return r.re.Exec(ctx, qCtx, ChainWalker{})
}

func ToExecutable(v any) Executable {
	switch v := v.(type) {
	case Executable:
		return v
	case RecursiveExecutable:
		return &reWrapper{re: v}
	default:
		return nil
	}
}
