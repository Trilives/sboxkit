#!/usr/bin/env bash
# 预下载 .deb 内捆绑的第三方资产（goreleaser nfpm 打包前执行）。每次执行都
# 强制重新下载最新稳定 sing-box，避免本地缓存混入新版本发布：
#   dist/deps/<arch>/sing-box          sing-box 内核（GPL-3.0，随包附 LICENSE）
#   dist/deps/rules/geosite-cn.srs     国内域名规则集（SagerNet/sing-geosite，GPL-3.0）
#   dist/deps/rules/geoip-cn.srs       国内 IP 规则集（SagerNet/sing-geoip，GPL-3.0）
#   dist/deps/LICENSE.sing-box         sing-box 上游许可证
set -euo pipefail

ARCHES=(${DEB_ARCHES:-amd64 arm64 armv7})
DEPS=packaging/deps
mkdir -p "${DEPS}/rules"

fetch() { curl -fL --retry 3 --retry-delay 2 -o "$2" "$1"; }

echo "==> sing-box 最新版本号"
TAG="$(curl -fsSL https://api.github.com/repos/SagerNet/sing-box/releases/latest | grep -oE '"tag_name":\s*"[^"]+"' | cut -d'"' -f4)"
VERSION="${TAG#v}"
if [[ -z "${TAG}" || -z "${VERSION}" ]]; then
  echo "无法获取最新 sing-box 稳定版版本号" >&2
  exit 1
fi
echo "    ${TAG}"

for arch in "${ARCHES[@]}"; do
  mkdir -p "${DEPS}/${arch}"
  echo "==> 强制下载 sing-box ${TAG} (${arch})"
  archive="${DEPS}/${arch}/sing-box.tar.gz"
  binary="${DEPS}/${arch}/sing-box"
  candidate="${binary}.new"
  rm -f "${archive}" "${candidate}"
  fetch "https://github.com/SagerNet/sing-box/releases/download/${TAG}/sing-box-${VERSION}-linux-${arch}.tar.gz" \
    "${archive}"
  tar -xzf "${archive}" -O "sing-box-${VERSION}-linux-${arch}/sing-box" > "${candidate}"
  rm -f "${archive}"
  chmod 0755 "${candidate}"
  # `go version -m` reads Go build metadata without executing the target, so
  # the same check works for arm64/armv7 binaries on the amd64 Actions runner.
  actual_version="$(go version -m "${candidate}" | awk '$1 == "mod" && $2 == "github.com/sagernet/sing-box" { print $3; exit }')"
  if [[ "${actual_version}" != "${TAG}" ]]; then
    echo "${arch}: sing-box 内核版本不匹配，期望 ${TAG}，实际 ${actual_version:-未知}" >&2
    rm -f "${candidate}"
    exit 1
  fi
  mv -f "${candidate}" "${binary}"
done

printf '%s\n' "${TAG}" > "${DEPS}/sing-box.version"

if [[ ! -s "${DEPS}/rules/geosite-cn.srs" ]]; then
  echo "==> 下载 geosite-cn.srs"
  fetch "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs" \
    "${DEPS}/rules/geosite-cn.srs"
fi

if [[ ! -s "${DEPS}/rules/geoip-cn.srs" ]]; then
  echo "==> 下载 geoip-cn.srs"
  fetch "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs" \
    "${DEPS}/rules/geoip-cn.srs"
fi

if [[ ! -s "${DEPS}/LICENSE.sing-box" ]]; then
  echo "==> 下载 sing-box LICENSE"
  fetch "https://raw.githubusercontent.com/SagerNet/sing-box/master/LICENSE" "${DEPS}/LICENSE.sing-box"
fi

echo "==> 完成"
ls -lh "${DEPS}"/*/sing-box "${DEPS}/sing-box.version" "${DEPS}/rules/" | sed 's/^/    /'
