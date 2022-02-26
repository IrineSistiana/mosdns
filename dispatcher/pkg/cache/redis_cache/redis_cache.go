//     Copyright (C) 2020-2021, IrineSistiana
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

package redis_cache

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/pool"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"math/rand"
	"sync/atomic"
	"time"
)

var nopLogger = zap.NewNop()

type RedisCache struct {
	// Client is the redis.Client. This must not be nil.
	Client *redis.Client

	// ClientTimeout specifies the timeout for read and write operations.
	// Default is 50ms.
	ClientTimeout time.Duration

	// Logger is the *zap.Logger for this RedisCache.
	// A nil Logger will disable logging.
	Logger *zap.Logger

	clientDisabled uint32
}

func (r *RedisCache) logger() *zap.Logger {
	if l := r.Logger; l != nil {
		return l
	}
	return nopLogger
}

func (r *RedisCache) clientTimeout() time.Duration {
	if t := r.ClientTimeout; t > 0 {
		return t
	}
	return time.Millisecond * 50
}

func (r *RedisCache) disabled() bool {
	return atomic.LoadUint32(&r.clientDisabled) != 0
}

func (r *RedisCache) disableClient() {
	if atomic.CompareAndSwapUint32(&r.clientDisabled, 0, 1) {
		r.logger().Warn("redis temporarily disabled")
		go func() {
			const maxBackoff = time.Second * 30
			backoff := time.Duration(0)
			for {
				if backoff >= maxBackoff {
					backoff = maxBackoff
				} else {
					backoff += time.Duration(rand.Intn(1000))*time.Millisecond + time.Second
				}
				time.Sleep(backoff)
				ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
				err := r.Client.Ping(ctx).Err()
				cancel()
				if err != nil {
					r.logger().Warn("redis ping failed", zap.Error(err))
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

	ctx, cancel := context.WithTimeout(context.Background(), r.clientTimeout())
	defer cancel()
	b, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		if err != redis.Nil {
			r.logger().Warn("redis get", zap.Error(err))
			r.disableClient()
		}
		return nil, time.Time{}, time.Time{}
	}

	storedTime, expirationTime, m, err := unpackRedisValue(b)
	if err != nil {
		r.logger().Warn("redis data unpack error", zap.Error(err))
		return nil, time.Time{}, time.Time{}
	}
	return m, storedTime, expirationTime
}

// Store stores kv into redis asynchronously.
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

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), r.clientTimeout())
		defer cancel()
		if err := r.Client.Set(ctx, key, data.Bytes(), ttl).Err(); err != nil {
			r.logger().Warn("redis set", zap.Error(err))
			r.disableClient()
		}
	}()
}

// Close closes the redis client.
func (r *RedisCache) Close() error {
	return r.Client.Close()
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
