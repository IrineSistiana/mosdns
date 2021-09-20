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

package plugin

// import all plugins
import (
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/arbitrary"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/blackhole"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/cache"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/ecs"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/fallback"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/fast_forward"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/forward"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/hosts"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/ipset"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/parallel"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/pipeline"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/sequence"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/sleep"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/ttl"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/matcher/query_matcher"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/matcher/response_matcher"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/server"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/strip"

	// v2ray proto data support
	_ "github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/v2data"
)
