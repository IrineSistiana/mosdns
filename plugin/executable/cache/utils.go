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
	"encoding/binary"
	"hash/maphash"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/cache"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/miekg/dns"
	"golang.org/x/exp/constraints"
)

type key string

var seed = maphash.MakeSeed()

const cacheKeyVersion = 1

func (k key) Sum() uint64 {
	return maphash.String(seed, string(k))
}

// getMsgKey returns a key for the query that is effective at the cache's
// current position in the sequence. It returns an empty string if the query
// should not be cached.
//
// The key includes the wire representation of the upstream query, minus its
// transaction ID. This accounts for the complete question, query flags, and
// EDNS options inserted by plugins before cache (notably ECS). Client EDNS is
// included separately because query_context keeps it outside Q(); this is
// required to distinguish the client DO bit and options that may be forwarded
// by a plugin placed after cache.
func getMsgKey(qCtx *query_context.Context) string {
	q := qCtx.Q()
	if q.Response || q.Opcode != dns.OpcodeQuery || len(q.Question) != 1 {
		return ""
	}

	keyQuery := q.Copy()
	keyQuery.Id = 0
	keyQuery.Compress = false
	queryWire, err := keyQuery.Pack()
	if err != nil {
		return ""
	}

	var clientOptWire []byte
	if clientOpt := qCtx.ClientOpt(); clientOpt != nil {
		// Pack the OPT inside a temporary DNS message instead of relying on an
		// option's String method. The DNS wire format is lossless for every
		// EDNS0 implementation supported by miekg/dns.
		m := &dns.Msg{Extra: []dns.RR{dns.Copy(clientOpt)}}
		clientOptWire, err = m.Pack()
		if err != nil {
			return ""
		}
	}

	// Length-prefixing makes the two wire messages unambiguous. Converting a
	// byte slice to string copies it, so the key remains valid after return.
	buf := make([]byte, 1+4+len(queryWire)+4+len(clientOptWire))
	buf[0] = cacheKeyVersion
	binary.BigEndian.PutUint32(buf[1:], uint32(len(queryWire)))
	offset := 5
	offset += copy(buf[offset:], queryWire)
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(clientOptWire)))
	offset += 4
	copy(buf[offset:], clientOptWire)
	return string(buf)
}

type item struct {
	resp           *dns.Msg
	storedTime     time.Time
	expirationTime time.Time
}

func copyNoOpt(m *dns.Msg) *dns.Msg {
	if m == nil {
		return nil
	}

	m2 := new(dns.Msg)
	m2.MsgHdr = m.MsgHdr
	m2.Compress = m.Compress

	if len(m.Question) > 0 {
		m2.Question = make([]dns.Question, len(m.Question))
		copy(m2.Question, m.Question)
	}

	lenExtra := len(m.Extra)
	for _, r := range m.Extra {
		if r.Header().Rrtype == dns.TypeOPT {
			lenExtra--
		}
	}

	s := make([]dns.RR, len(m.Answer)+len(m.Ns)+lenExtra)
	m2.Answer, s = s[:0:len(m.Answer)], s[len(m.Answer):]
	m2.Ns, s = s[:0:len(m.Ns)], s[len(m.Ns):]
	m2.Extra = s[:0:lenExtra]

	for _, r := range m.Answer {
		m2.Answer = append(m2.Answer, dns.Copy(r))
	}
	for _, r := range m.Ns {
		m2.Ns = append(m2.Ns, dns.Copy(r))
	}

	for _, r := range m.Extra {
		if r.Header().Rrtype == dns.TypeOPT {
			continue
		}
		m2.Extra = append(m2.Extra, dns.Copy(r))
	}
	return m2
}

// copyRespForCache keeps the response body separate from the context's EDNS
// handling, but saves the upstream OPT as the final OPT record. On a cache
// hit Context.SetResponse will move it back to UpstreamOpt, allowing wrappers
// such as ecs_handler and forward_edns0opt to reproduce their response-side
// EDNS processing.
func copyRespForCache(m *dns.Msg, upstreamOpt *dns.OPT) *dns.Msg {
	m2 := copyNoOpt(m)
	if upstreamOpt != nil {
		m2.Extra = append(m2.Extra, dns.Copy(upstreamOpt))
	}
	return m2
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

// getRespFromCache returns the cached response from cache.
// The ttl of returned msg will be changed properly.
// Returned bool indicates whether this response is hit by lazy cache.
// Note: Caller SHOULD change the msg id because it's not same as query's.
func getRespFromCache(msgKey string, backend *cache.Cache[key, *item], lazyCacheEnabled bool, lazyTtl int) (*dns.Msg, bool) {
	// Lookup cache
	v, _, _ := backend.Get(key(msgKey))

	// Cache hit
	if v != nil {
		now := time.Now()

		// Not expired.
		if now.Before(v.expirationTime) {
			r := v.resp.Copy()
			dnsutils.SubtractTTL(r, uint32(now.Sub(v.storedTime).Seconds()))
			return r, false
		}

		// Msg expired but cache isn't. This is a lazy cache enabled entry.
		// If lazy cache is enabled, return the response.
		if lazyCacheEnabled {
			r := v.resp.Copy()
			dnsutils.SetTTL(r, uint32(lazyTtl))
			return r, true
		}
	}

	// cache miss
	return nil, false
}

// saveRespToCache saves r to cache backend. It returns false if r
// should not be cached and was skipped.
func saveRespToCache(msgKey string, r *dns.Msg, upstreamOpt *dns.OPT, backend *cache.Cache[key, *item], lazyCacheTtl int) bool {
	if r.Truncated != false {
		return false
	}

	var msgTtl time.Duration
	var cacheTtl time.Duration
	switch r.Rcode {
	case dns.RcodeNameError:
		msgTtl = time.Second * 30
		cacheTtl = msgTtl
	case dns.RcodeServerFailure:
		msgTtl = time.Second * 5
		cacheTtl = msgTtl
	case dns.RcodeSuccess:
		minTTL := dnsutils.GetMinimalTTL(r)
		if len(r.Answer) == 0 { // Empty answer. Set ttl between 0~300.
			const maxEmtpyAnswerTtl = 300
			msgTtl = time.Duration(min(minTTL, maxEmtpyAnswerTtl)) * time.Second
			cacheTtl = msgTtl
		} else {
			msgTtl = time.Duration(minTTL) * time.Second
			if lazyCacheTtl > 0 {
				cacheTtl = time.Duration(lazyCacheTtl) * time.Second
			} else {
				cacheTtl = msgTtl
			}
		}
	}
	if msgTtl <= 0 || cacheTtl <= 0 {
		return false
	}

	now := time.Now()
	v := &item{
		resp:           copyRespForCache(r, upstreamOpt),
		storedTime:     now,
		expirationTime: now.Add(msgTtl),
	}
	backend.Store(key(msgKey), v, now.Add(cacheTtl))
	return true
}
