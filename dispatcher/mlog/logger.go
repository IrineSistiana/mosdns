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
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"sync/atomic"
	"unsafe"
)

type LogConfig struct {
	Level    string `yaml:"level"`
	File     string `yaml:"file"`
	ErrFile  string `yaml:"err_file"`
	InfoFile string `yaml:"info_file"`
}

var (
	atomicLevel      = zap.NewAtomicLevelAt(zap.InfoLevel)
	atomicInfoWriter = NewAtomicWriteSyncer(zapcore.Lock(os.Stdout))
	atomicErrWriter  = NewAtomicWriteSyncer(zapcore.Lock(os.Stderr))
	l                = newLogger(atomicLevel, atomicInfoWriter, atomicErrWriter)
	s                = l.Sugar()
)

func Level() zap.AtomicLevel {
	return atomicLevel
}

func InfoWriter() *AtomicWriteSyncer {
	return atomicInfoWriter
}

func ErrWriter() *AtomicWriteSyncer {
	return atomicErrWriter
}

func ApplyGlobalConfig(lc *LogConfig) error {
	_, err := handleLogConfig(lc, true)
	return err
}

func NewLogger(lc *LogConfig) (*zap.Logger, error) {
	return handleLogConfig(lc, false)
}

func handleLogConfig(lc *LogConfig, applyGlobally bool) (*zap.Logger, error) {
	var lvl zapcore.Level
	if len(lc.Level) > 0 {
		var ok bool
		lvl, ok = parseLogLevel(lc.Level)
		if !ok {
			return nil, fmt.Errorf("invalid log level [%s]", lc.Level)
		}
	}

	if len(lc.InfoFile) == 0 {
		lc.InfoFile = lc.File
	}
	if len(lc.ErrFile) == 0 {
		lc.ErrFile = lc.File
	}

	var infoWriter zapcore.WriteSyncer
	var errWriter zapcore.WriteSyncer

	if lf := lc.File; len(lf) > 0 {
		f, _, err := zap.Open(lf)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		fLocked := zapcore.Lock(f)
		infoWriter = fLocked
		errWriter = fLocked
	} else {
		if lf := lc.InfoFile; len(lf) > 0 {
			f, _, err := zap.Open(lf)
			if err != nil {
				return nil, fmt.Errorf("open info log file: %w", err)
			}
			infoWriter = zapcore.Lock(f)
		}
		if lf := lc.ErrFile; len(lf) > 0 {
			f, _, err := zap.Open(lf)
			if err != nil {
				return nil, fmt.Errorf("open err log file: %w", err)
			}
			errWriter = zapcore.Lock(f)
		}
	}

	if applyGlobally {
		if len(lc.Level) > 0 {
			atomicLevel.SetLevel(lvl)
		}
		if infoWriter != nil {
			atomicInfoWriter.Replace(infoWriter)
		}
		if errWriter != nil {
			atomicErrWriter.Replace(errWriter)
		}
		return nil, nil
	}

	var levelEnabler zapcore.LevelEnabler
	if len(lc.Level) == 0 {
		levelEnabler = atomicLevel
	} else {
		levelEnabler = lvl
	}
	if infoWriter == nil {
		infoWriter = atomicInfoWriter
	}
	if errWriter == nil {
		errWriter = atomicErrWriter
	}
	return newLogger(levelEnabler, infoWriter, errWriter), nil
}

func newLogger(lvl zapcore.LevelEnabler, infoWriter, errWriter zapcore.WriteSyncer) *zap.Logger {
	errLvl := zap.LevelEnablerFunc(func(l2 zapcore.Level) bool {
		return lvl.Enabled(l2) && l2 >= zapcore.ErrorLevel
	})
	infoLvl := zap.LevelEnablerFunc(func(l2 zapcore.Level) bool {
		return lvl.Enabled(l2) && l2 < zapcore.ErrorLevel
	})

	core := zapcore.NewTee(
		zapcore.NewCore(zapcore.NewConsoleEncoder(defaultEncoderConfig()), infoWriter, infoLvl),
		zapcore.NewCore(zapcore.NewConsoleEncoder(defaultEncoderConfig()), errWriter, errLvl),
	)
	return zap.New(core, zap.AddCaller())
}

// L returns a logger that has a lvl Level(), and will write
// logs to InfoWriter() and ErrWriter().
// The returned logger is shared by all L() call.
func L() *zap.Logger {
	return l
}

// S returns a sugared L.
// The returned logger is shared by all S() call.
func S() *zap.SugaredLogger {
	return s
}

// NewPluginLogger returns a named logger from L.
func NewPluginLogger(tag string) *zap.Logger {
	return l.Named(tag)
}

func defaultEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
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

func parseLogLevel(s string) (zapcore.Level, bool) {
	switch s {
	case "debug":
		return zap.DebugLevel, true
	case "", "info":
		return zap.InfoLevel, true
	case "warn":
		return zap.WarnLevel, true
	case "error":
		return zap.ErrorLevel, true
	default:
		return 0, false
	}
}
