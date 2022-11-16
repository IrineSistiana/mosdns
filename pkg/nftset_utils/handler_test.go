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

package nftset_utils

import (
	"github.com/google/nftables"
	"net/netip"
	"os"
	"testing"
)

func skipCI(t *testing.T) {
	if os.Getenv("TEST_NFTSET") == "" {
		t.SkipNow()
	}
}

func prepareSet(t testing.TB, tableName, setName string, interval bool) {
	t.Helper()
	nc, err := nftables.New()
	if err != nil {
		t.Fatal(err)
	}

	table := &nftables.Table{Name: tableName, Family: nftables.TableFamilyINet}
	nc.AddTable(table)
	if err := nc.AddSet(&nftables.Set{Name: setName, Table: table, KeyType: nftables.TypeIPAddr, Interval: interval}, nil); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
}

func Test_AddElems(t *testing.T) {
	skipCI(t)
	n := "test"
	prepareSet(t, n, n, false)

	nc, err := nftables.New()
	if err != nil {
		t.Fatal(err)
	}

	h := NewNtSetHandler(HandlerOpts{
		Conn:        nc,
		TableFamily: nftables.TableFamilyINet,
		TableName:   n,
		SetName:     n,
	})

	if err := h.AddElems(netip.MustParsePrefix("127.0.0.1/24")); err != nil {
		t.Fatal(err)
	}
	elems, err := nc.GetSetElements(h.set)
	if err != nil {
		t.Fatal(err)
	}
	if len(elems) == 0 {
		t.Fatal("set is empty")
	}
}
