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

package mmdb

import (
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider"
	"github.com/oschwald/geoip2-golang"
)

const PluginType = "mmdb"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

func Init(bp *coremain.BP, args any) (any, error) {
	return NewMmdb(bp, args.(*Args))
}

type Args struct {
	File string `yaml:"file"`
}

var _ data_provider.MmdbMatcherProvider = (*Mmdb)(nil)

type Mmdb struct {
	mmdb *geoip2.Reader
}

func (m *Mmdb) GetMmdbMatcher() *geoip2.Reader {
	return m.mmdb
}

func NewMmdb(bp *coremain.BP, args *Args) (*Mmdb, error) {
	m := &Mmdb{}

	db, err := geoip2.Open(args.File)
	if err == nil {
		m.mmdb = db
	}

	return m, nil
}
