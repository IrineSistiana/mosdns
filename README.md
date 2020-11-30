# mosdns

mosdns 是一个插件化配置的 DNS 转发器/服务器。每个插件实现一个功能。插件执行顺序可动态调整。

---

插件配置说明、教程和和示例等，详见：[wiki](https://github.com/IrineSistiana/mosdns/wiki)

下载预编译文件、更新日志，详见：[release](https://github.com/IrineSistiana/mosdns/releases)

---

目前支持的插件有:

- 功能性插件:
    - blackhole: 丢弃应答或返回拒绝性应答(屏蔽请求)。
    - ecs: 为请求添加 ECS。可以添加预设 IP，也可以根据客户端地址自动添加。
    - forward: 将请求送至服务器并获取应答。由 [AdGuardHome 的 dnsproxy 模块](https://github.com/AdguardTeam/dnsproxy) 驱动。支持 `AdguardHome` 支持的所有协议。
    - ipset: 添加应答中的 IP 到系统的 ipset 表。支持 `mask` 属性，可大幅减少表长度。
- 匹配器插件:
    - domain_matcher: 可以匹配请求和 CNAME 中的域名。支持从 v2ray 的 `geosite.dat` 文件加载数据。
    - ip_matcher: 匹配应答中的 IP。支持从 v2ray 的 `geoip.dat` 文件加载数据。
    - qtype_matcher: 匹配请求的类型。
- 路由插件:
    - sequence: 将插件连接起来。让插件按顺序执行，并且可动态调整执行顺序。

用户需要为每个插件起一个唯一的名字 `tag`。mosdns 使用 `tag` 来引用插件。

`sequence` 路由插件是 mosdns 的关键部分。使用类似 `if do else do` 的逻辑来连接其他插件。非常灵活，并且配置直观易理解。

比如，下面这个配置(片段)能实现根据域名和 IP 进行链式分流的策略。

```yaml
args:
    sequence:
        -   if:                         # 筛选出
                - '!match_qtype_A_AAAA' # 非 A AAAA 的请求。(qtype_matcher 插件)
                - match_local_domain    # 或已知的本地域名的请求。(domain_matcher 插件)
            exec:
                - forward_local         # 转发至本地服务器。(forward 插件)
            goto: end                   # 结束。

        -   exec:                       # 其余请求(不是已知的本地域名)。
                - forward_local         # 转发至本地服务器。
            sequence:
                -   if:                     # 筛选出
                        - match_local_ip    # 结果是本地 IP 的应答。(ip_matcher 插件)
                    goto: end               # 结束。

                                            # 其余请求(既不是已知的本地域名，应答也不含本地 IP)。
                -   exec:
                        - forward_remote    # 转发至远程服务器。
                    goto: end               # 结束。
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
