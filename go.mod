module github.com/IrineSistiana/mosdns/v3

go 1.17

require (
	github.com/AdguardTeam/dnsproxy v0.41.4
	github.com/go-redis/redis/v8 v8.11.4
	github.com/google/nftables v0.0.0-20220221214239-211824995dcb
	github.com/kardianos/service v1.2.1
	github.com/lucas-clemente/quic-go v0.25.0
	github.com/miekg/dns v1.1.47
	github.com/mitchellh/mapstructure v1.4.3
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	go.uber.org/zap v1.21.0
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20220310020820-b874c991c1a5
	google.golang.org/protobuf v1.27.1
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

replace github.com/lucas-clemente/quic-go v0.25.0 => github.com/IrineSistiana/quic-go v0.25.0-mod

require (
	github.com/AdguardTeam/golibs v0.10.7 // indirect
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/aead/poly1305 v0.0.0-20180717145839-3fee0db0b635 // indirect
	github.com/ameshkov/dnscrypt/v2 v2.2.3 // indirect
	github.com/ameshkov/dnsstamps v1.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cheekybits/genny v1.0.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/google/go-cmp v0.5.7 // indirect
	github.com/josharian/native v1.0.0 // indirect
	github.com/marten-seemann/qpack v0.2.1 // indirect
	github.com/marten-seemann/qtls-go1-16 v0.1.4 // indirect
	github.com/marten-seemann/qtls-go1-17 v0.1.0 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.0-beta.1 // indirect
	github.com/mdlayher/netlink v1.6.0 // indirect
	github.com/mdlayher/socket v0.2.2 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/vishvananda/netns v0.0.0-20211101163701-50045581ed74 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/crypto v0.0.0-20220307211146-efcb8507fb70 // indirect
	golang.org/x/mod v0.5.1 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.9 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
)
