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

package server

import (
	"crypto/tls"
	"errors"
	"net"
)

func (s *Server) ServeTLS(l net.Listener) error {
	var tlsConf *tls.Config
	if s.opts.TLSConfig != nil {
		tlsConf = s.opts.TLSConfig.Clone()
	} else {
		tlsConf = new(tls.Config)
	}

	if len(s.opts.Key)+len(s.opts.Cert) != 0 {
		cert, err := tls.LoadX509KeyPair(s.opts.Cert, s.opts.Key)
		if err != nil {
			return err
		}
		tlsConf.Certificates = append(tlsConf.Certificates, cert)
	}

	if len(tlsConf.Certificates) == 0 {
		return errors.New("missing certificate for tls listener")
	}

	l = tls.NewListener(l, tlsConf)
	return s.ServeTCP(l)
}
