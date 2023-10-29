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

// import all plugins
import (
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/arbitrary"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/blackhole"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/bufsize"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/cache"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/client_limiter"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/dual_selector"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/ecs"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/edns0_filter"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/fast_forward"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/forward"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/hosts"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/ipset"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/marker"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/metrics_collector"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/misc_optm"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/nftset"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/padding"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/query_summary"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/redirect"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/reverse_lookup"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/sequence"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/sleep"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/executable/ttl"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/matcher/query_matcher"
	_ "github.com/sieveLau/mosdns/v4-maintenance/plugin/matcher/response_matcher"
)
