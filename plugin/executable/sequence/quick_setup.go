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

// ExecQuickSetupFunc configures an Executable or
// RecursiveExecutable with a simple string args.
type ExecQuickSetupFunc func(bq BQ, args string) (any, error)

// MatchQuickSetupFunc configures a Matcher with a simple string args.
type MatchQuickSetupFunc func(bq BQ, args string) (Matcher, error)

var execQuickSetupReg struct {
	sync.RWMutex
	m map[string]ExecQuickSetupFunc
}

var matchQuickSetupReg struct {
	sync.RWMutex
	m map[string]MatchQuickSetupFunc
}

func RegExecQuickSetup(typ string, f ExecQuickSetupFunc) error {
	execQuickSetupReg.Lock()
	defer execQuickSetupReg.Unlock()

	_, ok := execQuickSetupReg.m[typ]
	if ok {
		return fmt.Errorf("type %s has already been registered", typ)
	}
	if execQuickSetupReg.m == nil {
		execQuickSetupReg.m = make(map[string]ExecQuickSetupFunc)
	}
	execQuickSetupReg.m[typ] = f
	return nil
}

func MustRegExecQuickSetup(typ string, f ExecQuickSetupFunc) {
	if err := RegExecQuickSetup(typ, f); err != nil {
		panic(err.Error())
	}
}

func GetExecQuickSetup(typ string) ExecQuickSetupFunc {
	execQuickSetupReg.RLock()
	defer execQuickSetupReg.RUnlock()
	return execQuickSetupReg.m[typ]
}

func RegMatchQuickSetup(typ string, f MatchQuickSetupFunc) error {
	matchQuickSetupReg.Lock()
	defer matchQuickSetupReg.Unlock()

	_, ok := matchQuickSetupReg.m[typ]
	if ok {
		return fmt.Errorf("type %s has already been registered", typ)
	}
	if matchQuickSetupReg.m == nil {
		matchQuickSetupReg.m = make(map[string]MatchQuickSetupFunc)
	}
	matchQuickSetupReg.m[typ] = f
	return nil
}

func MustRegMatchQuickSetup(typ string, f MatchQuickSetupFunc) {
	if err := RegMatchQuickSetup(typ, f); err != nil {
		panic(err.Error())
	}
}

func GetMatchQuickSetup(typ string) MatchQuickSetupFunc {
	matchQuickSetupReg.RLock()
	defer matchQuickSetupReg.RUnlock()
	return matchQuickSetupReg.m[typ]
}
