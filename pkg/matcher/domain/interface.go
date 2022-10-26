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

package domain

// "fqdn-insensitive" means the domain in Add() and Match() call
// is fqdn-insensitive. "google.com" and "google.com." will get
// the same outcome.
// The logic for case-insensitive is the same as above.

type Matcher[T any] interface {
	// Match matches the domain s.
	// s could be a fqdn or not, and should be case-insensitive.
	Match(s string) (v T, ok bool)
	Len() int
}

type WriteableMatcher[T any] interface {
	Matcher[T]
	Add(pattern string, v T) error
}
