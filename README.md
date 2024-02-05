# mosdns

功能概述、配置方式、教程等，详见: [wiki](https://irine-sistiana.gitbook.io/mosdns-wiki/)

下载预编译文件、更新日志，详见: [release](https://github.com/IrineSistiana/mosdns/releases)

docker 镜像: [sievelau/mosdns](https://hub.docker.com/r/sievelau/mosdns)

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

1. go, minimum version 1.20
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
