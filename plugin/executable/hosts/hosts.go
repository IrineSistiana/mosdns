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

package hosts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/hosts"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/go-chi/chi/v5"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"net/http"
	"os"
)

const PluginType = "hosts"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

var _ sequence.Executable = (*Hosts)(nil)

type Args struct {
	Entries []string `yaml:"entries"`
	Files   []string `yaml:"files"`
}

type Hosts struct {
	h *hosts.Hosts
}

func Init(bp *coremain.BP, args any) (any, error) {
	h, err := NewHosts(args.(*Args))
	bp.RegAPI(h.Api(bp.L()))
	return h, err
}

func NewHosts(args *Args) (*Hosts, error) {
	m := domain.NewMixMatcher[*hosts.IPs]()
	m.SetDefaultMatcher(domain.MatcherFull)
	for i, entry := range args.Entries {
		if err := domain.Load[*hosts.IPs](m, entry, hosts.ParseIPs); err != nil {
			return nil, fmt.Errorf("failed to load entry #%d %s, %w", i, entry, err)
		}
	}
	for i, file := range args.Files {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file #%d %s, %w", i, file, err)
		}
		if err := domain.LoadFromTextReader[*hosts.IPs](m, bytes.NewReader(b), hosts.ParseIPs); err != nil {
			return nil, fmt.Errorf("failed to load file #%d %s, %w", i, file, err)
		}
	}

	return &Hosts{
		h: hosts.NewHosts(m),
	}, nil
}

func (h *Hosts) Response(q *dns.Msg) *dns.Msg {
	return h.h.LookupMsg(q)
}

func (h *Hosts) Exec(_ context.Context, qCtx *query_context.Context) error {
	r := h.h.LookupMsg(qCtx.Q())
	if r != nil {
		qCtx.SetResponse(r)
	}
	return nil
}

func (h *Hosts) Api(logger *zap.Logger) *chi.Mux {
	router := chi.NewRouter()
	router.Post("/update", func(writer http.ResponseWriter, request *http.Request) {
		b := request.Body
		payload := map[string]interface{}{}
		payload["code"] = -1
		if err := domain.LoadFromTextReader[(*hosts.IPs)](h.h.GetMatcher().(*domain.MixMatcher[*hosts.IPs]), b, hosts.ParseIPs); err != nil {
			payload["msg"] = err.Error()
			if err := respondWithJSON(writer, http.StatusOK, payload); err != nil {
				logger.Error("fail to response hosts update", zap.Error(err))
			}
			return
		}
		payload["msg"] = "ok"
		payload["code"] = 0
		if err := respondWithJSON(writer, http.StatusOK, payload); err != nil {
			logger.Error("fail to response hosts update", zap.Error(err))
		}
	})
	return router
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) error {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := w.Write(response); err != nil {
		return err
	}
	return nil
}
