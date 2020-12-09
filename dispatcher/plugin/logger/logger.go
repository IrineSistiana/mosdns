//     Copyright (C) 2020, IrineSistiana
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

package logger

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"runtime"
	"time"
)

const PluginType = "logger"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

type logger struct {
	tag string
}

func (l *logger) Tag() string {
	return l.tag
}

func (l *logger) Type() string {
	return PluginType
}

type Args struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	if len(args.Level) != 0 {
		level, err := logrus.ParseLevel(args.Level)
		if err != nil {
			return nil, err
		}
		mlog.Logger().SetLevel(level)
	}
	if len(args.File) != 0 {
		f, err := os.OpenFile(args.File, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0755)
		if err != nil {
			return nil, fmt.Errorf("can not open log file %s: %w", args.File, err)
		}
		mlog.Entry().Infof("use log file %s", args.File)
		logWriter := io.MultiWriter(os.Stdout, f)
		mlog.Logger().SetOutput(logWriter)
	}
	if mlog.Logger().IsLevelEnabled(logrus.DebugLevel) {
		mlog.Logger().SetReportCaller(true)
		go func() {
			m := new(runtime.MemStats)
			for {
				time.Sleep(time.Second * 15)
				runtime.ReadMemStats(m)
				mlog.Entry().Debugf("HeapObjects: %d NumGC: %d PauseTotalNs: %d, NumGoroutine: %d", m.HeapObjects, m.NumGC, m.PauseTotalNs, runtime.NumGoroutine())
			}
		}()
	}

	return &logger{tag: tag}, nil
}
