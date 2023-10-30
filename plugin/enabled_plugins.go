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

// data providers
import (
	// data provider
	_ "github.com/IrineSistiana/mosdns/v5/plugin/data_provider/domain_set"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/data_provider/ip_set"

	// matcher
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/client_ip"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/cname"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/env"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/has_resp"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/has_wanted_ans"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/ptr_ip"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/qclass"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/qname"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/qtype"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/random"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/rcode"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/resp_ip"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/matcher/string_exp"

	// executable
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/arbitrary"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/black_hole"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/cache"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/debug_print"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/drop_resp"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/dual_selector"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/ecs_handler"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/forward"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/forward_edns0opt"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/hosts"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/ipset"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/metrics_collector"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/nftset"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/query_summary"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/rate_limiter"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/redirect"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/reverse_lookup"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence/fallback"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/sleep"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/executable/ttl"

	// executable and matcher
	_ "github.com/IrineSistiana/mosdns/v5/plugin/mark"

	// server
	_ "github.com/IrineSistiana/mosdns/v5/plugin/server/http_server"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/server/quic_server"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/server/tcp_server"
	_ "github.com/IrineSistiana/mosdns/v5/plugin/server/udp_server"
)
