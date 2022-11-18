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

package mlog

import (
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

type LogConfig struct {
	// Level, See also zapcore.ParseLevel.
	Level string `yaml:"level"`

	// File that logger will be writen into.
	// Default is stderr.
	File string `yaml:"file"`

	// Production enables json output.
	Production bool `yaml:"production"`

	// OmitTime omits the time in log.
	OmitTime bool `yaml:"omit_time"`

	// parsed level
	lvl zapcore.Level
}

var (
	stderr = zapcore.Lock(os.Stderr)

	lvl = zap.NewAtomicLevelAt(zap.InfoLevel)
	l   = newLogger(zapcore.NewConsoleEncoder, defaultEncoderConfig(), lvl, stderr)
	s   = l.Sugar()

	nop = zap.NewNop()
)

func NewLogger(lc *LogConfig) (*zap.Logger, error) {
	lvl, err := zapcore.ParseLevel(lc.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}
	lc.lvl = lvl

	return newLoggerFromCfg(lc)
}

func newLoggerFromCfg(lc *LogConfig) (*zap.Logger, error) {
	var out zapcore.WriteSyncer
	if lf := lc.File; len(lf) > 0 {
		f, _, err := zap.Open(lf)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		out = zapcore.Lock(f)
	} else {
		out = stderr
	}

	ec := defaultEncoderConfig()
	if lc.OmitTime {
		ec.TimeKey = ""
	}

	if lc.Production {
		return newLogger(zapcore.NewJSONEncoder, ec, lc.lvl, out), nil
	}
	return newLogger(zapcore.NewConsoleEncoder, ec, lc.lvl, out), nil
}

func newLogger(
	encoderFactory func(config zapcore.EncoderConfig) zapcore.Encoder,
	encoderCfg zapcore.EncoderConfig,
	lvl zapcore.LevelEnabler,
	out zapcore.WriteSyncer,
) *zap.Logger {
	core := zapcore.NewCore(encoderFactory(encoderCfg), out, lvl)
	return zap.New(core)
}

func L() *zap.Logger {
	return l
}

func SetLevel(l zapcore.Level) {
	lvl.SetLevel(l)
}

func S() *zap.SugaredLogger {
	return s
}

func Nop() *zap.Logger {
	return nop
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
