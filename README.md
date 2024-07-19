# mosdns

功能概述、配置方式、教程等，详见: [wiki](https://irine-sistiana.gitbook.io/mosdns-wiki/mosdns-v4)

*注意：wiki中的 `servers` - `listener` 应为 `listeners`。 Reminder: the configuration of `servers` should be `listeners` instead of `listener`.*

下载预编译文件，见: [release](https://github.com/sieveLau/mosdns/releases)

docker 镜像: [dockerhub: sievelau/mosdns](https://hub.docker.com/r/sievelau/mosdns)

## 配置文件结构/Configuration File Structure

```yaml
# 日志设置
log:
  level: info   # 日志级别。可选 "debug" "info" "warn" "error"。默认 "info"。
  file: "/path/to/log/file"      # 记录日志到文件。

# 从其他配置文件载入 include，数据源，插件和服务器设置
# include 的设置会比本配置文件中的设置先被初始化
include: []

# 数据源设置
data_providers:
  - tag: data1        # 数据源的 tag。由用户自由设定。不能重复。
    file: "/path/to/data/file"     # 文件位置
    auto_reload: false # 文件有变化时是否自动重载。

# 插件设置
plugins:
  - tag: tag1     # 插件的 tag。由用户自由设定。不能重复。
    type: type1   # 插件类型。详见下文。
    args:         # 插件参数。取决于插件类型。详见下文。
      key1: value1
      key2: value2

# 服务器设置
servers:
  - exec: plugin_tag1    # 本服务器运行插件的 tag。
    timeout: 5    # 请求处理超时时间。单位: 秒。默认: 5。
    listeners:     # 监听设置。是数组。可配置多个。
      - protocol: https           # 协议，支持 "udp", "tcp", "tls", "https" 和 "http"
        addr: ":443"              # 监听地址。
        cert: "/path/to/my/cert"  # TLS 所需证书文件。
        key: "/path/to/my/key"    # TLS 所需密钥文件。
        url_path: "/dns-query"    # DoH 路径。留空会跳过路径检查，任何请求路径会被处理。
        # DoH 从 HTTP 头获取用户 IP。需配合反向代理使用。(v4.3+) 配置后会屏蔽所有没有该头的请求。
        get_user_ip_from_header: "X-Forwarded-For"
        # (v4.3+) 启用 proxy protocol。需配合代理使用。UDP 服务器暂不支持。
        proxy_protocol: false
        idle_timeout: 10          # 连接复用空连接超时时间。单位: 秒。默认: 10。

      - protocol: udp
        addr: ":53"
      - protocol: tcp
        addr: ":53"
  # API 入口设置     
  api:
    http: "127.0.0.1:8080" # 在该地址启动 api 接口。
```

# 改动/Changes

现在对upstream新增了一个配置项`trust_ca`，可以指定一个CA文件的路径，该CA所颁发的证书在**该插件**的范围内会被信任；系统已经信任的证书也会被信任。例如：

Now the upstream plugin has a new option `trust_ca`, in which you can set the path to a CA cert which will be trusted in addition to those trusted by the OS. For example:

```yaml
plugins:
  - tag: ""
    type: "forward"
    args:
      upstream:
        - addr: "quic://192.168.1.1"
      trust_ca: "/etc/mosdns/rootCA.crt"
```

那么用`rootCA.crt`所签发的证书将会被信任。

Certificates issued by `rootCA.crt` will be trusted.

## Compile from source

Build dependencies:

1. go, minimum version 1.21
2. git, for cloning source code to local

Clone this repo:

```bash
git clone https://github.com/sieveLau/mosdns.git
```

Make a build directory:

```bash
cd mosdns
mkdir build
```

Then build:

```bash
cd build
go build ../
```

When the compilation is done, you will have a single `mosdns` executable in directory `build`.

# Todo

[ ] Create a wiki