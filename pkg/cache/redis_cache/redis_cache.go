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

package redis_cache

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/pool"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/utils"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"io"
	"math/rand"
	"sync/atomic"
	"time"
)

var nopLogger = zap.NewNop()

type RedisCacheOpts struct {
	// Client cannot be nil.
	Client redis.Cmdable

	// ClientCloser closes Client when RedisCache.Close is called.
	// Optional.
	ClientCloser io.Closer

	// ClientTimeout specifies the timeout for read and write operations.
	// Default is 50ms.
	ClientTimeout time.Duration

	// Logger is the *zap.Logger for this RedisCache.
	// A nil Logger will disable logging.
	Logger *zap.Logger
}

func (opts *RedisCacheOpts) Init() error {
	if opts.Client == nil {
		return errors.New("nil client")
	}
	utils.SetDefaultNum(&opts.ClientTimeout, time.Second)
	if opts.Logger == nil {
		opts.Logger = nopLogger
	}
	return nil
}

type RedisCache struct {
	opts           RedisCacheOpts
	clientDisabled uint32
}

func NewRedisCache(opts RedisCacheOpts) (*RedisCache, error) {
	if err := opts.Init(); err != nil {
		return nil, err
	}
	return &RedisCache{
		opts: opts,
	}, nil
}

func (r *RedisCache) disabled() bool {
	return atomic.LoadUint32(&r.clientDisabled) != 0
}

func (r *RedisCache) disableClient() {
	if atomic.CompareAndSwapUint32(&r.clientDisabled, 0, 1) {
		r.opts.Logger.Warn("redis temporarily disabled")
		go func() {
			const maxBackoff = time.Second * 30
			backoff := time.Millisecond * 100
			for {
				time.Sleep(backoff)
				ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
				err := r.opts.Client.Ping(ctx).Err()
				cancel()
				if err != nil {
					if backoff >= maxBackoff {
						backoff = maxBackoff
					} else {
						backoff += time.Duration(rand.Intn(1000))*time.Millisecond + time.Second
					}
					r.opts.Logger.Warn("redis ping failed", zap.Error(err), zap.Duration("next_ping", backoff))
					continue
				}
				atomic.StoreUint32(&r.clientDisabled, 0)
				return
			}
		}()
	}
}

func (r *RedisCache) Get(key string) (v []byte, storedTime, expirationTime time.Time) {
	if r.disabled() {
		return nil, time.Time{}, time.Time{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.opts.ClientTimeout)
	defer cancel()
	b, err := r.opts.Client.Get(ctx, key).Bytes()
	if err != nil {
		if err != redis.Nil {
			r.opts.Logger.Warn("redis get", zap.Error(err))
			r.disableClient()
		}
		return nil, time.Time{}, time.Time{}
	}

	storedTime, expirationTime, m, err := unpackRedisValue(b)
	if err != nil {
		r.opts.Logger.Warn("redis data unpack error", zap.Error(err))
		return nil, time.Time{}, time.Time{}
	}
	return m, storedTime, expirationTime
}

// Store stores kv into redis.
func (r *RedisCache) Store(key string, v []byte, storedTime, expirationTime time.Time) {
	if r.disabled() {
		return
	}

	now := time.Now()
	ttl := expirationTime.Sub(now)
	if ttl <= 0 { // For redis, zero ttl means the key has no expiration time.
		return
	}

	data := packRedisData(storedTime, expirationTime, v)
	defer data.Release()
	ctx, cancel := context.WithTimeout(context.Background(), r.opts.ClientTimeout)
	defer cancel()
	if err := r.opts.Client.Set(ctx, key, data.Bytes(), ttl).Err(); err != nil {
		r.opts.Logger.Warn("redis set", zap.Error(err))
		r.disableClient()
	}
}

type KV struct {
	Key            string
	V              []byte
	StoreTime      time.Time
	ExpirationTime time.Time
}

// BatchStore stores a batch of kv into redis via redis pipeline.
func (r *RedisCache) BatchStore(b []KV) {
	if r.disabled() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.opts.ClientTimeout)
	defer cancel()
	pipeline := r.opts.Client.Pipeline()
	buffers := make([]*pool.Buffer, 0, len(b))
	for _, kv := range b {
		now := time.Now()
		ttl := kv.ExpirationTime.Sub(now)
		if ttl <= 0 {
			continue
		}

		data := packRedisData(kv.StoreTime, kv.ExpirationTime, kv.V)
		buffers = append(buffers, data)
		pipeline.Set(ctx, kv.Key, data.Bytes(), ttl)
	}

	if _, err := pipeline.Exec(ctx); err != nil {
		r.opts.Logger.Warn("redis pipeline set", zap.Error(err))
		r.disableClient()
	}
	for _, buffer := range buffers {
		buffer.Release()
	}
}

// Close closes the redis client.
func (r *RedisCache) Close() error {
	if f := r.opts.ClientCloser; f != nil {
		return f.Close()
	}
	return nil
}

func (r *RedisCache) Len() int {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()
	i, err := r.opts.Client.DBSize(ctx).Result()
	if err != nil {
		r.opts.Logger.Error("dbsize", zap.Error(err))
		return 0
	}
	return int(i)
}

// packRedisData packs storedTime, expirationTime and v into one byte slice.
// The returned []byte should be released by pool.ReleaseBuf().
func packRedisData(storedTime, expirationTime time.Time, v []byte) *pool.Buffer {
	buf := pool.GetBuf(8 + 8 + len(v))
	b := buf.Bytes()
	binary.BigEndian.PutUint64(b[:8], uint64(storedTime.Unix()))
	binary.BigEndian.PutUint64(b[8:16], uint64(expirationTime.Unix()))
	copy(b[16:], v)
	return buf
}

func unpackRedisValue(b []byte) (storedTime, expirationTime time.Time, v []byte, err error) {
	if len(b) < 16 {
		return time.Time{}, time.Time{}, nil, errors.New("b is too short")
	}
	storedTime = time.Unix(int64(binary.BigEndian.Uint64(b[:8])), 0)
	expirationTime = time.Unix(int64(binary.BigEndian.Uint64(b[8:16])), 0)
	return storedTime, expirationTime, b[16:], nil
}
