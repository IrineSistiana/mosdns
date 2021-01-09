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

package utils

import (
	"sync"
	"time"
)

var (
	timerPool = sync.Pool{}
)

func GetTimer(t time.Duration) *time.Timer {
	timer, ok := timerPool.Get().(*time.Timer)
	if !ok {
		return time.NewTimer(t)
	}
	if timer.Reset(t) {
		panic("dispatcher.go getTimer: active timer trapped in timerPool")
	}
	return timer
}

func ReleaseTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timerPool.Put(timer)
}

func ResetAndDrainTimer(timer *time.Timer, d time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(d)
}
