package simple

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"go.uber.org/zap"
)

var (
	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSHandshakeTimeout: time.Second * 5,
			IdleConnTimeout:     time.Second * 60,
			MaxIdleConns:        5,
			MaxIdleConnsPerHost: 5,
			DisableCompression:  true,
		},
	}
)

func (m *simpleServer) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) (err error) {
	record := recordPool.Get().(*record)
	defer record.release()

	record.SetQuery(qCtx)

	err = next.ExecNext(ctx, qCtx)
	record.Err = err

	if r := qCtx.R(); r != nil {
		record.SetResp(qCtx)
	}

	bd, _ := json.Marshal(record)
	go m.push(bd)
	return
}

func (m *simpleServer) push(d []byte) {
	if m.backend.Len() > m.args.Size {
		_ = m.backend.Remove(m.backend.Front())
	}
	m.backend.PushBack(d)
	m.send(d)
}

func (m *simpleServer) send(d []byte) (err error) {
	if m.args.WebHook == "" {
		return
	}
	resp, err := httpClient.Post(m.args.WebHook, "application/json", bytes.NewBuffer(d))
	if err != nil {
		m.logger.Debug("HTTP request failed", zap.Error(err))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		m.logger.Debug("Failed to read response", zap.Error(err))
		return
	}
	defer resp.Body.Close() // Close the response body.
	if resp.StatusCode > 299 {
		m.logger.Debug("WebHook sent incorrect data", zap.String("status", resp.Status), zap.String("body", string(body)))
	}
	return
}
