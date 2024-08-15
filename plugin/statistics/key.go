package statistics

import "github.com/IrineSistiana/mosdns/v5/pkg/query_context"

var (
	ArbitraryStoreKey = query_context.RegKey()
	HostStoreKey      = query_context.RegKey()
	ForwardStoreKey   = query_context.RegKey()
	FallbackStoreKey  = query_context.RegKey()
	CacaheStoreKey    = query_context.RegKey()
)
