# mosdns

功能概述、配置方式、教程等，详见: [wiki](https://irine-sistiana.gitbook.io/mosdns-wiki/)

下载预编译文件、更新日志，详见: [release](https://github.com/IrineSistiana/mosdns/releases)

docker 镜像: [sievelau/mosdns](https://hub.docker.com/r/sievelau/mosdns)

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
