//go:build linux

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
	"fmt"
	"github.com/google/nftables"
	"go4.org/netipx"
	"net/netip"
	"sync"
	"time"
)

// NftSetHandler can add netip.Prefix to the corresponding set.
// The table that contains this set must be an inet family table.
// If the set has a 'interval' flag, the prefix from netip.Prefix will be
// applied.
type NftSetHandler struct {
	opts  HandlerOpts
	table *nftables.Table

	m          sync.Mutex
	lastUpdate time.Time
	set        *nftables.Set
}

type HandlerOpts struct {
	Conn        *nftables.Conn // Required.
	TableFamily nftables.TableFamily
	TableName   string // Required
	SetName     string // Required
}

// NewNtSetHandler inits NftSetHandler.
func NewNtSetHandler(opts HandlerOpts) *NftSetHandler {
	table := &nftables.Table{
		Name:   opts.TableName,
		Family: opts.TableFamily,
	}
	return &NftSetHandler{
		opts:  opts,
		table: table,
	}
}

// getSet get set info from kernel. It has an internal cache and won't
// invoke a syscall every time.
func (h *NftSetHandler) getSet() (*nftables.Set, error) {
	const refreshInterval = time.Second

	now := time.Now()
	h.m.Lock()
	defer h.m.Unlock()
	if h.set != nil && now.Sub(h.lastUpdate) < refreshInterval {
		return h.set, nil
	}
	set, err := h.opts.Conn.GetSetByName(h.table, h.opts.SetName)
	if err != nil {
		return nil, err
	}
	h.set = set
	h.lastUpdate = now
	return set, nil
}

// AddElems adds SetIPElem(s) to set in a single batch.
func (h *NftSetHandler) AddElems(es ...netip.Prefix) error {
	set, err := h.getSet()
	if err != nil {
		return fmt.Errorf("failed to get set, %w", err)
	}

	var elems []nftables.SetElement
	if set.Interval {
		elems = make([]nftables.SetElement, 0, 2*len(es))
	} else {
		elems = make([]nftables.SetElement, 0, len(es))
	}

	for _, e := range es {
		if set.Interval && !e.IsSingleIP() {
			r := netipx.RangeOfPrefix(e)
			start := r.From()
			end := r.To()
			elems = append(
				elems,
				nftables.SetElement{Key: start.AsSlice(), IntervalEnd: false},
				nftables.SetElement{Key: end.Next().AsSlice(), IntervalEnd: true},
			)
		} else {
			elems = append(elems, nftables.SetElement{Key: e.Addr().AsSlice()})
		}
	}
	err = h.opts.Conn.SetAddElements(set, elems)
	if err != nil {
		return err
	}
	return h.opts.Conn.Flush()
}
