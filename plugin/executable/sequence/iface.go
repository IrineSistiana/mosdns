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

// RecursiveExecutable represents something that is executable and requires stack.
type RecursiveExecutable interface {
	Exec(ctx context.Context, qCtx *query_context.Context, next ChainWalker) error
}

// Executable represents something that is executable.
type Executable interface {
	Exec(ctx context.Context, qCtx *query_context.Context) error
}

// Matcher represents a matcher that can match a certain patten in Context.
type Matcher interface {
	Match(ctx context.Context, qCtx *query_context.Context) (bool, error)
}

type RecursiveExecutableFunc func(ctx context.Context, qCtx *query_context.Context, next ChainWalker) error

func (f RecursiveExecutableFunc) Exec(ctx context.Context, qCtx *query_context.Context, next ChainWalker) error {
	return f(ctx, qCtx, next)
}

type ExecutableFunc func(ctx context.Context, qCtx *query_context.Context) error

func (f ExecutableFunc) Exec(ctx context.Context, qCtx *query_context.Context) error {
	return f(ctx, qCtx)
}

type MatchFunc func(ctx context.Context, qCtx *query_context.Context) (bool, error)

func (f MatchFunc) Match(ctx context.Context, qCtx *query_context.Context) (bool, error) {
	return f(ctx, qCtx)
}
