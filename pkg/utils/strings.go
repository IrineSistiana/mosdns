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

package utils

import (
	"strings"
	"unsafe"
)

// BytesToStringUnsafe converts bytes to string.
func BytesToStringUnsafe(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// RemoveComment removes comment after "symbol".
func RemoveComment(s, symbol string) string {
	if i := strings.Index(s, symbol); i >= 0 {
		return s[:i]
	}
	return s
}

// SplitString2 split s to two parts by given symbol
func SplitString2(s, symbol string) (s1 string, s2 string, ok bool) {
	if len(symbol) == 0 {
		return "", s, true
	}
	if i := strings.Index(s, symbol); i >= 0 {
		return s[:i], s[i+len(symbol):], true
	}
	return "", "", false
}
