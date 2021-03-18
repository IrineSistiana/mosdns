//     Copyright (C) 2020-2021, IrineSistiana
//
//     This key is part of mosdns.
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

package load_cache

import (
	"strconv"
	"sync"
	"sync/atomic"
)

var globalCache = NewCache()

func GetCache() *Cache {
	return globalCache
}

type Cache struct {
	sync.RWMutex
	m map[string]interface{}

	nn uint32
}

func NewCache() *Cache {
	return &Cache{m: make(map[string]interface{})}
}

func (c *Cache) Get(key string) (interface{}, bool) {
	c.RLock()
	defer c.RUnlock()

	v, ok := c.m[key]
	return v, ok
}

func (c *Cache) Store(key string, v interface{}) {
	c.Lock()
	defer c.Unlock()

	c.m[key] = v
}

func (c *Cache) Remove(key string) {
	c.Lock()
	defer c.Unlock()

	delete(c.m, key)
}

func (c *Cache) Purge() {
	c.Lock()
	defer c.Unlock()

	c.m = make(map[string]interface{})
}

func (c *Cache) NewNamespace() *NNCache {
	return &NNCache{
		prefix: strconv.Itoa(int(atomic.AddUint32(&c.nn, 1))),
		c:      c,
	}
}

type NNCache struct {
	prefix string
	c      *Cache
}

func (c *NNCache) combineKey(key string) string {
	return c.prefix + key
}

func (c *NNCache) Store(key string, v interface{}) {
	c.c.Store(c.combineKey(key), v)
}

func (c *NNCache) Remove(key string) {
	c.c.Remove(c.combineKey(key))
}

func (c *NNCache) Get(key string) (interface{}, bool) {
	return c.c.Get(c.combineKey(key))
}
