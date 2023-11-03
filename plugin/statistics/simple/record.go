package simple

import (
	"sync"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/statistics"
	"github.com/miekg/dns"
)

var recordPool = sync.Pool{
	New: func() interface{} {
		return &record{}
	},
}

type record struct {
	ID       uint32
	StartAt  time.Time
	ClientIP string

	IsArbitrary bool   `json:",omitempty"`
	IsHost      bool   `json:",omitempty"`
	ForwardName string `json:",omitempty"`
	Fallback    uint   `json:",omitempty"`
	CacheID     uint16 `json:",omitempty"`

	Consuming string

	Op     string `json:",omitempty"`
	Status string `json:",omitempty"`

	QOpt      string `json:",omitempty"`
	ClientOpt string `json:",omitempty"`
	Query     map[string][]any

	RespOpt     string           `json:",omitempty"`
	UpstreamOpt string           `json:",omitempty"`
	Resp        map[string][]any `json:",omitempty"`

	Err error `json:",omitempty"`
}

func (m *record) release() {
	m.IsArbitrary = false
	m.IsHost = false
	m.ForwardName = ""
	m.Fallback = 0
	m.CacheID = 0

	m.QOpt = ""
	m.ClientOpt = ""

	recordPool.Put(m)
}

func (m *record) SetQuery(qCtx *query_context.Context) {
	m.ID = qCtx.Id()
	m.StartAt = qCtx.StartTime()
	m.ClientIP = qCtx.ServerMeta.ClientAddr.String()
	if qCtx.QOpt() != nil {
		m.QOpt = qCtx.QOpt().String()
	}
	if qCtx.ClientOpt() != nil {
		m.ClientOpt = qCtx.ClientOpt().String()
	}

	m.Query = makeDnsMsg(qCtx.Q())
}

func (m *record) SetResp(qCtx *query_context.Context) {
	m.Resp = makeDnsMsg(qCtx.R())

	if qCtx.RespOpt() != nil {
		m.RespOpt = qCtx.RespOpt().String()
	}
	if qCtx.ClientOpt() != nil {
		m.ClientOpt = qCtx.ClientOpt().String()
	}

	if isArbitrary, has := qCtx.GetValue(statistics.ArbitraryStoreKey); has {
		m.IsArbitrary = isArbitrary.(bool)
	}
	if isHost, has := qCtx.GetValue(statistics.HostStoreKey); has {
		m.IsHost = isHost.(bool)
	}
	if forwardName, has := qCtx.GetValue(statistics.ForwardStoreKey); has {
		m.ForwardName = forwardName.(string)
	}
	if fallback, has := qCtx.GetValue(statistics.FallbackStoreKey); has {
		m.Fallback = fallback.(uint)
	}
	if cacheID, has := qCtx.GetValue(statistics.CacaheStoreKey); has {
		m.CacheID = cacheID.(uint16)
	}

	m.Consuming = time.Since(qCtx.StartTime()).String()

	m.Status = dns.RcodeToString[qCtx.R().Rcode]
	m.Op = dns.OpcodeToString[qCtx.R().Opcode]
}

func makeHflags(h dns.MsgHdr) (flags []any) {
	flags = []any{}
	if h.Response {
		flags = append(flags, "qr")
	}
	if h.Authoritative {
		flags = append(flags, "aa")
	}
	if h.Truncated {
		flags = append(flags, "tc")
	}
	if h.RecursionDesired {
		flags = append(flags, "rd")
	}
	if h.RecursionAvailable {
		flags = append(flags, "ra")
	}
	if h.Zero { // Hmm
		flags = append(flags, "z")
	}
	if h.AuthenticatedData {
		flags = append(flags, "ad")
	}
	if h.CheckingDisabled {
		flags = append(flags, "aa")
	}
	return
}

func makeDnsMsg(m *dns.Msg) (data map[string][]any) {
	data = make(map[string][]any)
	data["flags"] = makeHflags(m.MsgHdr)
	if len(m.Question) > 0 {
		data["Question"] = []any{}
		for _, q := range m.Question {
			data["Question"] = append(data["Question"], makeQuestion(q))
		}
	}
	if len(m.Answer) > 0 {
		data["Answer"] = []any{}
		for _, q := range m.Answer {
			data["Answer"] = append(data["Answer"], makeRR(q))
		}
	}
	if len(m.Ns) > 0 {
		data["Ns"] = []any{}
		for _, ns := range m.Ns {
			data["Ns"] = append(data["Ns"], makeRR(ns))
		}
	}
	if len(m.Extra) > 0 {
		data["Extra"] = []any{}
		for _, extra := range m.Extra {
			data["Extra"] = append(data["Extra"], makeRR(extra))
		}
	}
	return
}

func makeQuestion(q dns.Question) map[string]string {
	return map[string]string{
		"Name":  q.Name,
		"Type":  dns.TypeToString[q.Qtype],
		"Class": dns.ClassToString[q.Qclass],
	}
}

func makeRR(q dns.RR) map[string]any {
	h := q.Header()
	return map[string]any{
		"name":   h.Name,
		"ttl":    h.Ttl,
		"Type":   dns.TypeToString[h.Rrtype],
		"Class":  dns.ClassToString[h.Class],
		"Header": h.String(),
		"value":  q.String(),
	}
}
