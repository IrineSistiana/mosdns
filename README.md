# mosdns

mosdns 是一个插件化配置的 DNS 转发器/服务器。每个插件实现一个功能。插件执行顺序可动态调整。

---

教程、配置说明、插件示例等，详见：[wiki](https://github.com/IrineSistiana/mosdns/wiki)

下载预编译文件、更新日志，详见：[release](https://github.com/IrineSistiana/mosdns/releases)

---

目前支持的插件有:

- 功能性插件:
    - blackhole: 丢弃应答或返回拒绝性应答(屏蔽请求)。
    - ecs: 为请求添加 ECS。可以添加预设 IP，也可以根据客户端地址自动添加。
    - forward: 将请求送至服务器并获取应答。由 [AdGuardHome 的 dnsproxy 模块](https://github.com/AdguardTeam/dnsproxy) 驱动。支持 `AdGuardHome` 支持的所有协议。
    - ipset: 添加应答中的 IP 到系统的 ipset 表。支持 `mask` 属性，可大幅减少表长度。
- 匹配器插件:
    - domain_matcher: 可以匹配请求和 CNAME 中的域名。支持从 v2ray 的 `geosite.dat` 文件加载数据。
    - ip_matcher: 匹配应答中的 IP。支持从 v2ray 的 `geoip.dat` 文件加载数据。
    - qtype_matcher: 匹配请求的类型。
- 路由插件:
    - sequence: 将插件连接起来。让插件按顺序执行，并且可动态调整执行顺序。
    
--- 

配置方式:

用户定义好功能性插件和匹配器插件，然后为每一个插件起一个唯一的易理解的名字，也就是 `tag`。也就是说每一个 `tag` 都对应一个插件。

用户选择路由插件，将插件组合起来，实现不同功能。

目前 mosdns 唯一的路由插件: `sequence`。支持 `if` 条件判断、嵌套、重定向。非常灵活，并且配置直观易理解。

比如，下面这个 `sequence` 的配置将不同插件用 `tag` 组合起来，实现了非 A AAAA 的请求另行处理、屏蔽广告域名、屏蔽Bogus NXDomain IP、域名和 IP 分流。

```yaml
args:
  exec:

    # 非 A AAAA 的请求另行处理
    - if:
        - "!match_qtype_A_AAAA" # 匹配非 A AAAA 的请求(插件类型: qtype_matcher)
      exec:
        - forward_local         # 转发至本地服务器(插件类型: forward)
      goto: end                 # 结束(不再执行后续插件。立即返回应答至客户端)

    # 屏蔽广告域名
    - if:
        - match_ad_domain       # 匹配已知的广告域名(插件类型: domain_matcher)
      exec:
        - block                 # 屏蔽(插件类型: blackhole)
      goto: end                 # 结束

    - add_ecs                   # 添加 ECS(插件类型: ecs)
    - forward_local             # 转发至本地服务器

    # 屏蔽包含了 Bogus NXDomain IP 的应答
    - if:
        - match_nx_ip         # 匹配包含 Bogus NXDomain IP 应答。(插件类型: ip_matcher)
      exec:
        - block_with_nxdomain # 返回 NXDOMAIN
      goto: end               # 结束

    # 过滤出本地结果
    - if:
        - match_chn_domain      # 匹配已知的本地域名
        - match_chn_ip          # 或应答包含本地 IP
      goto: end                 # 结束

    # 剩余请求(既不是已知的本地域名，也没有返回本地 IP)
    - forward_remote    # 转发至远程服务器
  next: end             # 结束
```

## Open Source Components / Libraries / Reference

依赖

* [sirupsen/logrus](https://github.com/sirupsen/logrus): [MIT](https://github.com/sirupsen/logrus/blob/master/LICENSE)
* [miekg/dns](https://github.com/miekg/dns): [LICENSE](https://github.com/miekg/dns/blob/master/LICENSE)
* [go-yaml/yaml](https://github.com/go-yaml/yaml): [Apache License 2.0](https://github.com/go-yaml/yaml/blob/v2/LICENSE)
* [v2fly/v2ray-core](https://github.com/v2fly/v2ray-core): [MIT](https://github.com/v2fly/v2ray-core/blob/master/LICENSE)
* [vishvananda/netlink](https://github.com/vishvananda/netlink): [Apache License 2.0](https://github.com/vishvananda/netlink/blob/master/LICENSE)
* [AdguardTeam/dnsproxy](https://github.com/AdguardTeam/dnsproxy): [GPLv3](https://github.com/AdguardTeam/dnsproxy/blob/master/LICENSE)
* [mitchellh/mapstructure](https://github.com/mitchellh/mapstructure): [MIT](https://github.com/mitchellh/mapstructure/blob/master/LICENSE)

使用源码

* [xtaci/smux](https://github.com/xtaci/smux): [MIT](https://github.com/xtaci/smux/blob/master/LICENSE)
