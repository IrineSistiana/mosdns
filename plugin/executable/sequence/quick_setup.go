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
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"go.uber.org/zap"
	"sync"
)

type BQ interface {
	M() *coremain.Mosdns
	L() *zap.Logger
}

type bq struct {
	m *coremain.Mosdns
	l *zap.Logger
}

func (bq *bq) M() *coremain.Mosdns {
	return bq.m
}

func (bq *bq) L() *zap.Logger {
	return bq.l
}

func NewBQ(m *coremain.Mosdns, l *zap.Logger) BQ {
	return &bq{m: m, l: l}
}

// QuickSetupFunc configures an Executable or RecursiveExecutable or Matcher.
// with a simple string args.
type QuickSetupFunc func(bq BQ, args string) (any, error)

var quickSetupReg struct {
	sync.RWMutex
	m map[string]QuickSetupFunc
}

func RegQuickSetup(typ string, f QuickSetupFunc) error {
	quickSetupReg.Lock()
	defer quickSetupReg.Unlock()

	_, ok := quickSetupReg.m[typ]
	if ok {
		return fmt.Errorf("type %s has already been registered", typ)
	}
	if quickSetupReg.m == nil {
		quickSetupReg.m = make(map[string]QuickSetupFunc)
	}
	quickSetupReg.m[typ] = f
	return nil
}

func MustRegQuickSetup(typ string, f QuickSetupFunc) {
	if err := RegQuickSetup(typ, f); err != nil {
		panic(err.Error())
	}
}

func GetQuickSetup(typ string) QuickSetupFunc {
	quickSetupReg.RLock()
	defer quickSetupReg.RUnlock()
	return quickSetupReg.m[typ]
}
