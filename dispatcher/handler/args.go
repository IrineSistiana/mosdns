//     Copyright (C) 2020, IrineSistiana
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

package handler

import (
	"fmt"
	"github.com/mitchellh/mapstructure"
)

// Args contains plugin arguments.
type Args map[string]interface{}

type ArgError struct {
	key string
}

func NewArgErr(key string) error {
	return &ArgError{key: key}
}

func (err *ArgError) Error() string {
	return fmt.Sprintf("argument key [%s] is not defined or invalid", err.key)
}

func (a Args) Load(key string) (value interface{}, ok bool) {
	value, ok = a[key]
	return
}

func (a Args) LoadString(key string) (value string, ok bool) {
	if i, ok := a[key]; ok {
		value, ok = i.(string)
		return value, ok
	}
	return
}

func (a Args) LoadInt(key string) (value int, ok bool) {
	if i, ok := a[key]; ok {
		value, ok = i.(int)
		return value, ok
	}
	return
}

func (a Args) LoadBool(key string) (ok bool) {
	if i, ok := a[key]; ok {
		if v, ok := i.(bool); ok {
			return v
		}
	}
	return
}

func (a Args) LoadSlice(key string) (s []interface{}, ok bool) {
	if i, ok := a[key]; ok {
		if v, ok := i.([]interface{}); ok {
			return v, true
		}
	}
	return
}

func (a Args) WeakDecode(output interface{}) error {
	config := &mapstructure.DecoderConfig{
		Metadata:         nil,
		Result:           output,
		WeaklyTypedInput: true,
		TagName:          "yaml",
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(a)
}
