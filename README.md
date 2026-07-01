# mosdns

功能概述、配置方式、教程等，详见: [wiki](https://irine-sistiana.gitbook.io/mosdns-wiki/)

下载预编译文件、更新日志，详见: [release](https://github.com/IrineSistiana/mosdns/releases)

docker 镜像: [docker hub](https://hub.docker.com/r/irinesistiana/mosdns)


## 补充配置示例

### mmdb / geoip / geosite

`mmdb`、`geoip`、`geosite` 都是数据提供插件，需要先配置数据文件，再在 `sequence` 中通过对应 matcher 引用。

- `mmdb` 使用 MaxMind mmdb 数据库，配合 `resp_ip_mmdb` 按响应 IP 的国家/地区 ISO code 匹配。
- `geoip` 使用 v2ray `geoip.dat`，配合 `resp_ip_geoip` 按响应 IP 所属列表匹配。
- `geosite` 使用 v2ray `geosite.dat`，配合 `qname_geosite` 按请求域名所属列表匹配，支持 geosite attr，例如 `cn@cn`。

示例：

```yaml
plugins:
  - tag: mmdb
    type: mmdb
    args:
      file: /etc/mosdns/GeoLite2-Country.mmdb

  - tag: geoip
    type: geoip
    args:
      file: /etc/mosdns/geoip.dat

  - tag: geosite
    type: geosite
    args:
      file: /etc/mosdns/geosite.dat

  - tag: forward_local
    type: forward
    args:
      upstreams:
        - addr: 223.5.5.5

  - tag: forward_remote
    type: forward
    args:
      upstreams:
        - addr: tls://8.8.8.8

  - tag: main_sequence
    type: sequence
    args:
      - matches:
          - qname_geosite $geosite cn
        exec: forward_local

      - matches:
          - qname_geosite $geosite cn@cn
        exec: forward_local

      - matches:
          - resp_ip_geoip $geoip cn
        exec: forward_local

      - matches:
          - resp_ip_mmdb $mmdb CN
        exec: forward_local

      - exec: forward_remote
```

### TCP Fast Open

TCP Fast Open 通过 `enable_tfo: true` 开启。该选项只影响 TCP 类 socket；在不支持 TFO 的系统上不会生效。Linux 环境还需要系统内核和 `net.ipv4.tcp_fastopen` 允许对应方向的 TFO。

Server 示例：

```yaml
plugins:
  - tag: tcp_server
    type: tcp_server
    args:
      entry: main_sequence
      listen: 0.0.0.0:53
      enable_tfo: true

  - tag: http_server
    type: http_server
    args:
      entries:
        - path: /dns-query
          exec: main_sequence
      listen: 0.0.0.0:443
      cert: /etc/mosdns/server.crt
      key: /etc/mosdns/server.key
      enable_tfo: true
```

Upstream 示例：

```yaml
plugins:
  - tag: forward_tfo
    type: forward
    args:
      enable_tfo: true
      upstreams:
        - addr: tcp://8.8.8.8
        - addr: tls://1.1.1.1

  - tag: forward_tfo_mixed
    type: forward
    args:
      upstreams:
        - addr: tcp://8.8.4.4
          enable_tfo: true
        - addr: https://dns.google/dns-query
          enable_tfo: true
```
