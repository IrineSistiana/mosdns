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

package handler

import "context"

func wrapPluginBeforeReg(gp Plugin) Plugin {
	switch p := gp.(type) {
	case MatcherPlugin:
		return &matcherPluginWrapper{MatcherPlugin: p}
	case ContextPlugin:
		return &contextPluginWrapper{ContextPlugin: p}
	case ExecutablePlugin:
		return &executablePluginWrapper{ExecutablePlugin: p}
	default:
		return gp
	}
}

type executablePluginWrapper struct {
	ExecutablePlugin
}

func (w *executablePluginWrapper) Exec(ctx context.Context, qCtx *Context) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}

	err = w.ExecutablePlugin.Exec(ctx, qCtx)
	if err != nil {
		return NewPluginError(w.ExecutablePlugin.Tag(), err)
	}
	return nil
}

type contextPluginWrapper struct {
	ContextPlugin
}

func (w *contextPluginWrapper) Connect(ctx context.Context, qCtx *Context, pipeCtx *PipeContext) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}

	err = w.ContextPlugin.Connect(ctx, qCtx, pipeCtx)
	if err != nil {
		return NewPluginError(w.ContextPlugin.Tag(), err)
	}
	return nil
}

type matcherPluginWrapper struct {
	MatcherPlugin
}

func (w *matcherPluginWrapper) Match(ctx context.Context, qCtx *Context) (matched bool, err error) {
	if err = ctx.Err(); err != nil {
		return false, err
	}

	matched, err = w.MatcherPlugin.Match(ctx, qCtx)
	if err != nil {
		return false, NewPluginError(w.MatcherPlugin.Tag(), err)
	}
	return matched, nil
}
