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
	"sync"
)

type Errors struct {
	sync.RWMutex
	es []error
}

func (c *Errors) Error() string {
	c.Lock()
	defer c.Unlock()

	switch len(c.es) {
	case 0:
		return ""
	case 1:
		return c.es[0].Error()
	}

	b := new(strings.Builder)
	b.WriteString("multi errors:")
	b.WriteString(c.es[0].Error())
	for _, e := range c.es[1:] {
		b.WriteString(", ")
		b.WriteString(e.Error())
	}
	return b.String()
}

func (c *Errors) Append(err error) {
	c.Lock()
	defer c.Unlock()
	c.es = append(c.es, err)
}

func (c *Errors) Len() int {
	c.Lock()
	defer c.Unlock()
	return len(c.es)
}
