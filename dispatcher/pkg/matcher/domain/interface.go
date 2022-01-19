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

package domain

type Matcher interface {
	// Match matches the domain s.
	// s could be a fqdn or not, and should be case-insensitive.
	Match(s string) (v interface{}, ok bool)
	Len() int
	Add(pattern string, v interface{}) error
}

type Appendable interface {
	// Append appends v to Appendable.
	// v might be any type and nil.
	Append(v interface{})
}
