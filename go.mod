module github.com/IrineSistiana/mosdns

go 1.15

require (
	github.com/AdguardTeam/dnsproxy v0.33.7
	github.com/ameshkov/dnscrypt/v2 v2.0.2 // indirect
	github.com/golang/protobuf v1.4.3
	github.com/miekg/dns v1.1.35
	github.com/mitchellh/mapstructure v1.4.0
	github.com/sirupsen/logrus v1.7.0
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	go.starlark.net v0.0.0-20201210151846-e81fc95f7bd5 // indirect
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad // indirect
	golang.org/x/net v0.0.0-20201224014010-6772e930b67b
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a
	golang.org/x/sys v0.0.0-20201223074533-0d417f636930
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	v2ray.com/core v4.19.1+incompatible
)

// this version isn't correct
replace v2ray.com/core => github.com/v2fly/v2ray-core v0.0.0-20201225111350-8c5b392f2763
