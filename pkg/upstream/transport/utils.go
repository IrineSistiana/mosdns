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

package transport

import (
	"encoding/binary"
	"io"

	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/exp/constraints"
)

const (
	dnsHeaderLen = 12 // minimum dns msg size
)

func copyMsgWithLenHdr(m []byte) (*[]byte, error) {
	l := len(m)
	if l > dns.MaxMsgSize {
		return nil, ErrPayloadOverFlow
	}
	bp := pool.GetBuf(l + 2)
	binary.BigEndian.PutUint16(*bp, uint16(l))
	copy((*bp)[2:], m)
	return bp, nil
}

func copyMsg(m []byte) *[]byte {
	bp := pool.GetBuf(len(m))
	copy((*bp), m)
	return bp
}

// readMsgUdp reads dns frame from r. r typically should be a udp connection.
// It uses a 4kb rx buffer and ignores any payload that is too small for a dns msg.
// If no error, the length of payload always >= 12 bytes.
func readMsgUdp(r io.Reader) (*[]byte, error) {
	// TODO: Make this configurable?
	// 4kb should be enough.
	payload := pool.GetBuf(4095)

readAgain:
	n, err := r.Read(*payload)
	if err != nil {
		pool.ReleaseBuf(payload)
		return nil, err
	}
	if n < dnsHeaderLen {
		goto readAgain
	}
	*payload = (*payload)[:n]
	return payload, err
}

func setDefaultGZ[T constraints.Float | constraints.Integer](i *T, s, d T) {
	if s > 0 {
		*i = s
	} else {
		*i = d
	}
}

var nopLogger = zap.NewNop()

func setNonNilLogger(i **zap.Logger, s *zap.Logger) {
	if s != nil {
		*i = s
	} else {
		*i = nopLogger
	}
}
