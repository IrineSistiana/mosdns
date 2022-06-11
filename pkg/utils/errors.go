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
	"fmt"
	"strings"
)

type Errors []error

func (es *Errors) Error() string {
	return es.String()
}

func (es *Errors) Append(err error) {
	*es = append(*es, err)
}

func (es *Errors) Build() error {
	switch len(*es) {
	case 0:
		return nil
	case 1:
		return (*es)[0]
	default:
		return es
	}
}

func (es *Errors) String() string {
	sb := new(strings.Builder)
	sb.WriteString("joint errors:")
	for i, err := range *es {
		sb.WriteString(fmt.Sprintf(" #%d: %v", i, err))
	}
	return sb.String()
}
