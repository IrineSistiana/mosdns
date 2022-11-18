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

package cache

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"google.golang.org/protobuf/proto"
	"io"
	"time"
)

func (c *Cache[K, V]) Dump(
	marshalKey func(k K) (string, error),
	marshalValue func(v V) ([]byte, error),
) ([]byte, error) {
	now := time.Now()
	cd := new(CacheDump)
	rangeFunc := func(k K, v V, storedTime, expirationTime time.Time) {
		if expirationTime.Before(now) {
			return
		}
		kb, err := marshalKey(k)
		if err != nil {
			return
		}
		vb, err := marshalValue(v)
		if err != nil {
			return
		}
		cd.Entries = append(cd.Entries, &CachedEntry{
			Key:            kb,
			StoreTime:      storedTime.Unix(),
			ExpirationTime: expirationTime.Unix(),
			Value:          vb,
		})
	}
	c.Range(rangeFunc)

	b, err := proto.Marshal(cd)
	if err != nil {
		return nil, fmt.Errorf("failed to pack cache dump, %w", err)
	}
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	_, _ = gw.Write(b)
	_ = gw.Close()

	return buf.Bytes(), nil
}

func (c *Cache[K, V]) LoadDump(
	b []byte,
	unmarshalKey func(s string) (K, error),
	unmarshalValue func(b []byte) (V, error),
) error {
	uncompress := func(b []byte) ([]byte, error) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}

		b, err = io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		if err := r.Close(); err != nil { // Close validates the gzip checksum.
			return nil, err
		}
		return b, nil
	}

	b, err := uncompress(b)
	if err != nil {
		return fmt.Errorf("failed to uncompress dump, %w", err)
	}

	cd := new(CacheDump)
	if err := proto.Unmarshal(b, cd); err != nil {
		return err
	}
	for _, e := range cd.Entries {
		k, err := unmarshalKey(e.Key)
		if err != nil {
			return fmt.Errorf("failed to unmarshal key, %w", err)
		}
		v, err := unmarshalValue(e.Value)
		if err != nil {
			return fmt.Errorf("failed to unmarshal value, %w", err)
		}
		c.Store(k, v, time.Unix(e.StoreTime, 0), time.Unix(e.ExpirationTime, 0))
	}
	return nil
}
