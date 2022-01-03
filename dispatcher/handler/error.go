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
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/mlog"
	"go.uber.org/zap"
)

type PluginError struct {
	tag string
	err error
}

func NewPluginError(tag string, err error) *PluginError {
	return &PluginError{tag: tag, err: err}
}

func (e *PluginError) Error() string {
	return fmt.Sprintf("%s: %v", e.tag, e.err)
}

func (e *PluginError) Unwrap() error {
	return e.err
}

func (e *PluginError) Is(target error) bool {
	return errors.Is(e.err, target)
}

// PluginFatalErr: If a plugin has a fatal err, call this.
func PluginFatalErr(tag string, msg string) {
	mlog.L().Fatal("plugin fatal err", zap.String("from", tag), zap.String("msg", msg))
}
