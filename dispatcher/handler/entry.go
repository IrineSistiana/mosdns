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
	"context"
	"errors"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

// dispatch sends qCtx to entries and return its first valid result.
func (r *entryRegister) dispatch(ctx context.Context, qCtx *Context) error {
	entries := r.get()
	if len(entries) == 0 {
		return errors.New("empty entry")
	}

	if len(entries) == 1 {
		return dispatchSingleEntry(ctx, qCtx, entries[0])
	}
	return dispatchMultiEntries(ctx, qCtx, entries)
}

func dispatchMultiEntries(ctx context.Context, qCtx *Context, entries []string) error {
	resChan := make(chan *dns.Msg, 1)
	upstreamWG := sync.WaitGroup{}
	for _, entry := range entries {
		entry := entry
		upstreamWG.Add(1)
		go func() {
			defer upstreamWG.Done()

			entryQCtx := qCtx.Copy() // qCtx cannot be modified in different goroutine. Copy it.

			queryStart := time.Now()
			err := Walk(ctx, entryQCtx, entry)
			rtt := time.Since(queryStart).Milliseconds()
			if err != nil {
				if err != context.Canceled {
					qCtx.Logf(logrus.WarnLevel, "entry %s returned an err after %dms: %v", entry, rtt, err)
				}
				return
			}

			if entryQCtx.R != nil {
				qCtx.Logf(logrus.DebugLevel, "reply from entry %s accepted, rtt: %dms", entry, rtt)
				select {
				case resChan <- entryQCtx.R:
				default:
				}
			}
		}()
	}

	entriesFailedNotificationChan := make(chan struct{}, 0)
	// this go routine notifies the Dispatch if all entries are failed
	go func() {
		// all entries are returned
		upstreamWG.Wait()
		// avoid below select{} choose entriesFailedNotificationChan
		// if both resChan and entriesFailedNotificationChan are selectable
		if len(resChan) == 0 {
			close(entriesFailedNotificationChan)
		}
	}()

	select {
	case m := <-resChan:
		qCtx.R = m
		return nil
	case <-entriesFailedNotificationChan:
		return errors.New("all entries failed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func dispatchSingleEntry(ctx context.Context, qCtx *Context, entry string) error {
	queryStart := time.Now()
	err := Walk(ctx, qCtx, entry)
	rtt := time.Since(queryStart).Milliseconds()
	qCtx.Logf(logrus.DebugLevel, "entry %s returned after %dms:", entry, rtt)
	return err
}
