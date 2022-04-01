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

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/mlog"
	"go.uber.org/zap"
)

var (
	_ Executable = (*PluginWrapper)(nil)
	_ Matcher    = (*PluginWrapper)(nil)
)

// PluginWrapper wraps the original plugin to avoid extremely frequently
// interface conversion.
type PluginWrapper struct {
	Plugin
	l *zap.Logger
	e Executable
	m Matcher
}

func NewPluginWrapper(gp Plugin) *PluginWrapper {
	w := new(PluginWrapper)
	w.Plugin = gp

	if se, ok := gp.(Executable); ok {
		w.e = se
	}
	if m, ok := gp.(Matcher); ok {
		w.m = m
	}
	if v, ok := gp.(interface{ L() *zap.Logger }); ok {
		w.l = v.L()
	}

	return w
}

func (w *PluginWrapper) Match(ctx context.Context, qCtx *Context) (matched bool, err error) {
	matched, err = w.match(ctx, qCtx)
	if err != nil {
		return false, NewPluginError(w.Tag(), err)
	}
	w.getLogger().Debug("matching query context", qCtx.InfoField(), zap.String("tag", w.Tag()), zap.Bool("result", matched))
	return matched, nil
}

func (w *PluginWrapper) match(ctx context.Context, qCtx *Context) (matched bool, err error) {
	if err = ctx.Err(); err != nil {
		return false, err
	}

	if w.m == nil {
		return false, fmt.Errorf("plugin tag: %s, type: %s is not a Matcher", w.Tag(), w.Type())
	}

	return w.m.Match(ctx, qCtx)
}

func (w *PluginWrapper) Exec(ctx context.Context, qCtx *Context, next ExecutableChainNode) error {
	w.getLogger().Debug("executing plugin", qCtx.InfoField(), zap.String("tag", w.Tag()))
	err := w.exec(ctx, qCtx, next)
	if err != nil {
		return NewPluginError(w.Tag(), err)
	}
	return nil
}

func (w *PluginWrapper) exec(ctx context.Context, qCtx *Context, next ExecutableChainNode) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if w.e == nil {
		return fmt.Errorf("plugin tag: %s, type: %s is not an ESExecutable nor Executable", w.Tag(), w.Type())
	}

	return w.e.Exec(ctx, qCtx, next)
}

func (w *PluginWrapper) getLogger() *zap.Logger {
	if w.l != nil {
		return w.l
	}
	return mlog.L()
}

type PluginInterfaceType uint8

const (
	PITESExecutable PluginInterfaceType = iota
	PITMatcher
)

func (w *PluginWrapper) Is(t PluginInterfaceType) bool {
	switch t {
	case PITESExecutable:
		return w.e != nil
	case PITMatcher:
		return w.m != nil
	default:
		panic(fmt.Sprintf("hander: invalid PluginInterfaceType: %d", t))
	}
}
