module github.com/IrineSistiana/mosdns

go 1.15

require (
	github.com/AdguardTeam/dnsproxy v0.33.2
	github.com/AdguardTeam/golibs v0.4.4 // indirect
	github.com/ameshkov/dnscrypt/v2 v2.0.1 // indirect
	github.com/golang/protobuf v1.4.3
	github.com/joomcode/errorx v1.0.3 // indirect
	github.com/lucas-clemente/quic-go v0.19.2 // indirect
	github.com/miekg/dns v1.1.35
	github.com/mitchellh/mapstructure v1.4.0
	github.com/sirupsen/logrus v1.7.0
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	go.starlark.net v0.0.0-20201118183435-e55f603d8c79 // indirect
	golang.org/x/crypto v0.0.0-20201124201722-c8d3bf9c5392 // indirect
	golang.org/x/net v0.0.0-20201201195509-5d6afe98e0b7
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/sys v0.0.0-20201201145000-ef89a241ccb3
	golang.org/x/text v0.3.4 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	v2ray.com/core v4.19.1+incompatible
)

// this version isn't correct
replace v2ray.com/core => github.com/v2fly/v2ray-core v0.0.0-20201023173911-0dc17643a07c
