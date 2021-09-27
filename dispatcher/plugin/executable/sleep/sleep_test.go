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

package sleep

import (
	"context"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"testing"
	"time"
)

func Test_sleep_sleep(t *testing.T) {

	tests := []struct {
		name       string
		ctxTimeout time.Duration
		d          time.Duration
		wantErr    bool
	}{
		{"ctx timeout", 0, time.Second, true},
		{"timer fired", time.Second, time.Millisecond, false},
		{"no-op", time.Second, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sleep{
				BP: handler.NewBP(tt.name, PluginType),
				d:  tt.d,
			}
			ctx, cancel := context.WithTimeout(context.Background(), tt.ctxTimeout)
			defer cancel()

			if err := s.sleep(ctx); (err != nil) != tt.wantErr {
				t.Errorf("sleep() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
