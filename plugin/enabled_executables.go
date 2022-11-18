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

package plugin

import (
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/arbitrary"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/black_hole"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/cache"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/drop_resp"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/dual_selector"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/ecs"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/fast_forward"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/forward"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/hosts"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/ipset"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/metrics_collector"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/misc_optm"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/nftset"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/query_summary"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/redirect"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/reverse_lookup"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence/fallback"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/sleep"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/ttl"
)
