# mosdns

mosdns 是一个插件化配置的 DNS 转发器/服务器。每个插件实现一个功能。插件执行顺序可动态调整。

---

插件配置说明、教程和和示例等，详见：[wiki](https://github.com/IrineSistiana/mosdns/wiki)

下载预编译文件、更新日志，详见：[release](https://github.com/IrineSistiana/mosdns/releases)

---

有的插件能执行特定操作，有的插件能匹配请求的特征。有的插件能将其他插件连接起来。每个插件都有一个由用户定义的唯一的 `tag`。

mosdns 支持使用类似 `if do else do` 的逻辑来连接插件。使用由用户定义的 `tag` 来引用插件。使配置更灵活简洁易理解。

比如，下面这个配置(片段)实现了根据域名和 IP 进行链式分流的策略。

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
