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

package string_exp

import (
	"context"
	"os"
	"testing"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestMatcher_Match(t *testing.T) {
	r := require.New(t)
	q := new(dns.Msg)
	qc := query_context.NewContext(q)
	qc.ServerMeta = query_context.ServerMeta{UrlPath: "/dns-query", ServerName: "a.b.c"}
	os.Setenv("STRING_EXP_TEST", "abc")

	doTest := func(arg string, want bool) {
		t.Helper()
		urlMatcher, err := QuickSetupFromStr(arg)
		r.NoError(err)
		got, err := urlMatcher.Match(context.Background(), qc)
		r.NoError(err)
		r.Equal(want, got)
	}

	doTest("url_path zl", false)
	doTest("url_path eq /dns-query", true)
	doTest("url_path eq /123 /dns-query /abc", true)
	doTest("url_path eq /123 /abc", false)
	doTest("url_path contains abc dns def", true)
	doTest("url_path contains abc def", false)
	doTest("url_path prefix abc /dns def", true)
	doTest("url_path prefix abc def", false)
	doTest("url_path suffix abc query def", true)
	doTest("url_path suffix abc def", false)
	doTest("url_path regexp ^/dns-query$", true)
	doTest("url_path regexp ^abc", false)

	doTest("server_name eq abc a.b.c def", true)
	doTest("server_name eq abc def", false)

	doTest("$STRING_EXP_TEST eq 123 abc def", true)
	doTest("$STRING_EXP_TEST eq 123 def", false)
	doTest("$STRING_EXP_TEST_NOT_EXIST eq 123 abc def", false)
	doTest("$STRING_EXP_TEST_NOT_EXIST zl", true)
}
