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

package redis_cache

import (
	"reflect"
	"testing"
	"time"
)

func Test_RedisValue(t *testing.T) {
	tests := []struct {
		name           string
		storedTime     time.Time
		expirationTime time.Time
		m              []byte
	}{
		{"test", time.Now(), time.Now().Add(time.Second), make([]byte, 1024)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := packRedisData(tt.storedTime, tt.expirationTime, tt.m)
			defer data.Release()

			storedTime, expirationTime, m, err := unpackRedisValue(data.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if storedTime.Unix() != tt.storedTime.Unix() {
				t.Fatalf("storedTime: want %v, got %v", tt.storedTime, storedTime)
			}
			if expirationTime.Unix() != tt.expirationTime.Unix() {
				t.Fatalf("expirationTime: want %v, got %v", tt.expirationTime, expirationTime)
			}
			if !reflect.DeepEqual(m, tt.m) {
				t.Fatalf("m: want %v, got %v", tt.m, m)
			}
		})
	}
}
