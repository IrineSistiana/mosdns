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
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/google/nftables"
	"go4.org/netipx"
)

var (
	ErrClosed = errors.New("closed handler")
)

// NftSetHandler can add netip.Prefix to the corresponding set.
// The table that contains this set must be an inet family table.
// If the set has a 'interval' flag, the prefix from netip.Prefix will be
// applied.
type NftSetHandler struct {
	opts HandlerOpts

	m           sync.Mutex
	closed      bool
	lastUpdate  time.Time
	set         *nftables.Set
	lastingConn *nftables.Conn // Note: lasting conn is not concurrent safe so m is required.

	disableSetCache bool // for test only
}

type HandlerOpts struct {
	TableFamily nftables.TableFamily
	TableName   string
	SetName     string
}

// NewNtSetHandler inits NftSetHandler.
func NewNtSetHandler(opts HandlerOpts) *NftSetHandler {
	return &NftSetHandler{
		opts: opts,
	}
}

// getSetLocked get set info from kernel. It has an internal cache and won't
// invoke a syscall every time.
func (h *NftSetHandler) getSetLocked() (*nftables.Set, error) {
	const refreshInterval = time.Second

	now := time.Now()
	if !h.disableSetCache && h.set != nil && now.Sub(h.lastUpdate) < refreshInterval {
		return h.set, nil
	}

	// Note: GetSetByName is not concurrent safe.
	set, err := h.lastingConn.GetSetByName(&nftables.Table{Name: h.opts.TableName, Family: h.opts.TableFamily}, h.opts.SetName)
	if err != nil {
		return nil, err
	}
	h.set = set
	h.lastUpdate = now
	return set, nil
}

// AddElems adds netip.Prefix(s) to set in a single batch.
func (h *NftSetHandler) AddElems(es ...netip.Prefix) error {
	h.m.Lock()
	defer h.m.Unlock()

	if h.closed {
		return ErrClosed
	}

	if h.lastingConn == nil {
		c, err := nftables.New(nftables.AsLasting())
		if err != nil {
			return fmt.Errorf("failed to open netlink, %w", err)
		}
		h.lastingConn = c
	}

	set, err := h.getSetLocked()
	if err != nil {
		return fmt.Errorf("failed to get set, %w", err)
	}

	var elems []nftables.SetElement
	if set.Interval {
		elems = make([]nftables.SetElement, 0, 2*len(es))
	} else {
		elems = make([]nftables.SetElement, 0, len(es))
	}

	for i, e := range es {
		if !e.IsValid() {
			return fmt.Errorf("invalid prefix at index %d", i)
		}
		if set.Interval {
			start := e.Masked().Addr()
			elems = append(elems, nftables.SetElement{Key: start.AsSlice(), IntervalEnd: false})
			
			end := netipx.PrefixLastIP(e).Next() // may be invalid if end is overflowed
			if end.IsValid() {
				elems = append(elems, nftables.SetElement{Key: end.AsSlice(), IntervalEnd: true})
			}
		} else {
			elems = append(elems, nftables.SetElement{Key: e.Addr().AsSlice()})
		}
	}

	err = h.lastingConn.SetAddElements(set, elems)
	if err != nil {
		return err
	}
	return h.lastingConn.Flush()
}

func (h *NftSetHandler) Close() error {
	h.m.Lock()
	defer h.m.Unlock()

	if h.closed {
		return nil
	}

	h.closed = true
	if h.lastingConn != nil {
		return h.lastingConn.CloseLasting()
	}
	return nil
}
