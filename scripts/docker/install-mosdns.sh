#!/bin/sh
set -ex

# mosdns needs this to verify tls certificates
apk add --no-cache ca-certificates

# Get ARCH
PLATFORM="$1"
TAG="$2"
REPOSITORY="$3"

if [ -z "$PLATFORM" ]; then
  ARCH="amd64"
else
  case "$PLATFORM" in
  linux/amd64)
    ARCH="amd64"
    ;;
  linux/arm/v7)
    ARCH="arm-7"
    ;;
  linux/arm64)
    ARCH="arm64"
    ;;
  *)
    ARCH=""
    ;;
  esac
fi
[ -z "${ARCH}" ] && echo "Invalid PLATFORM: ${PLATFORM}" && exit 1

mkdir mosdns_temp
cd mosdns_temp

# Download files
MOSDNS_BIN="mosdns-linux-${ARCH}.zip"
ASSET_URL="https://github.com/${REPOSITORY}/releases/download/${TAG}/${MOSDNS_BIN}"

wget -O ./mosdns.zip "${ASSET_URL}" >/dev/null 2>&1

unzip mosdns.zip
chmod +x mosdns
mv mosdns /usr/bin/

mkdir /etc/mosdns

cd ..
rm -rf ./mosdns_temp
