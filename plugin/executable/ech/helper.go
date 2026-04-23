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

package ech

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/forward"
	"github.com/miekg/dns"
	"io"
	"unsafe"
)

func validateECH(bs []byte) error {
	r := bytes.NewReader(bs)

	var rl uint16
	err := binary.Read(r, binary.BigEndian, &rl)
	if err != nil {
		return err
	}

	eh := struct {
		Version uint16
		Length  uint16
	}{0, 0}

	i := uint16(2)
	for i < rl {
		err = binary.Read(r, binary.BigEndian, &eh)
		if err != nil {
			return err
		}

		if eh.Version != 0xfe0d {
			return fmt.Errorf("unsupported ECH version: %#x", eh.Version)
		}

		_, err = r.Seek(int64(eh.Length), io.SeekCurrent)
		if err != nil {
			return err
		}

		i += uint16(unsafe.Sizeof(eh)) + eh.Length
	}

	return nil
}

func validateECHString(s string) error {
	bs, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return validateECH(bs)
}

func resolveECH(fw *fastforward.Forward, qn string) (*dns.HTTPS, *dns.SVCBECHConfig, uint32, error) {
	m := dns.Msg{}
	m.SetQuestion(qn, dns.TypeHTTPS)

	qc := query_context.NewContext(&m)
	err := fw.Exec(context.Background(), qc)
	if err != nil {
		return nil, nil, 0, err
	}

	r := qc.R()
	if r == nil {
		return nil, nil, 0, fmt.Errorf("resolve ECH failed: nil R()")
	}

	for _, rr := range r.Answer {
		if rr.Header().Rrtype != dns.TypeHTTPS {
			continue
		}
		ht := rr.(*dns.HTTPS)
		for _, kv := range ht.Value {
			if kv.Key() != dns.SVCB_ECHCONFIG {
				continue
			}
			ec := kv.(*dns.SVCBECHConfig)
			err := validateECH(ec.ECH)
			if err == nil {
				return ht, ec, ht.Hdr.Ttl, nil
			} else {
				return nil, nil, 0, fmt.Errorf("resolve ECH failed: invalid ECHConfigList: %w", err)
			}
		}
	}

	return nil, nil, 0, fmt.Errorf("resolve ECH failed: no ECHConfigList")
}
