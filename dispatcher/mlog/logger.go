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

package mlog

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"sync/atomic"
	"unsafe"
)

var (
	atomicLevel  = zap.NewAtomicLevelAt(zap.InfoLevel)
	atomicWriter = NewAtomicWriteSyncer(os.Stderr)
	l            = defaultLogger()
	s            = l.Sugar()
)

func Level() zap.AtomicLevel {
	return atomicLevel
}

func Writer() *AtomicWriteSyncer {
	return atomicWriter
}

func L() *zap.Logger {
	return l
}

func S() *zap.SugaredLogger {
	return s
}

func NewPluginLogger(tag string) *zap.Logger {
	return l.Named(tag)
}

func defaultLogger() *zap.Logger {
	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		MessageKey:     "msg",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderCfg), atomicWriter, atomicLevel)
	return zap.New(core, zap.AddCaller())
}

type AtomicWriteSyncer struct {
	ws unsafe.Pointer
}

func NewAtomicWriteSyncer(ws zapcore.WriteSyncer) *AtomicWriteSyncer {
	return &AtomicWriteSyncer{ws: unsafe.Pointer(&ws)}
}

func (a *AtomicWriteSyncer) Replace(ws zapcore.WriteSyncer) {
	atomic.StorePointer(&a.ws, unsafe.Pointer(&ws))
}

func (a *AtomicWriteSyncer) Write(p []byte) (n int, err error) {
	ws := *(*zapcore.WriteSyncer)(atomic.LoadPointer(&a.ws))
	return ws.Write(p)
}

func (a *AtomicWriteSyncer) Sync() error {
	ws := *(*zapcore.WriteSyncer)(atomic.LoadPointer(&a.ws))
	return ws.Sync()
}
