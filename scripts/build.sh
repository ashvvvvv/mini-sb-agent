#!/bin/bash
# 标准构建：主构建 + 六构建矩阵 + md5 锚点
# 用法：./scripts/build.sh          全部构建
#       ./scripts/build.sh main    仅主构建（生产二进制）
# 必须在仓库主目录执行（build ID 含路径，其他目录构建 md5 不同）
set -euo pipefail
cd "$(dirname "$0")/.."

# Linux 生产机的 Go 不在默认 PATH 时自愈
if [ -x /usr/local/go/bin/go ] && ! command -v go >/dev/null 2>&1; then
    PATH=/usr/local/go/bin:$PATH
fi

TARGET="${1:-all}"

build_main() {
    echo "==> 主构建（with_utls，生产用）"
    go build -tags with_utls -o mini-sb-agent ./cmd/mini-sb-agent
    md5sum mini-sb-agent
}

build_matrix() {
    echo "==> 六构建矩阵"
    mkdir -p build
    go build -tags 'minimal with_vless' -o build/vless-direct ./cmd/mini-sb-agent
    go build -tags 'minimal with_vless with_shadowsocks_outbound' -o build/vless-ss ./cmd/mini-sb-agent
    go build -tags 'minimal with_hysteria2' -o build/hy2-direct ./cmd/mini-sb-agent
    go build -tags 'minimal with_hysteria2 with_shadowsocks_outbound' -o build/hy2-ss ./cmd/mini-sb-agent
    go build -tags 'minimal with_vless with_hysteria2' -o build/vless-hy2-direct ./cmd/mini-sb-agent
    go build -tags 'minimal with_vless with_hysteria2 with_shadowsocks_outbound' -o build/vless-hy2-ss ./cmd/mini-sb-agent
    echo "==> capabilities 自检"
    for b in build/vless-direct build/vless-ss build/hy2-direct build/hy2-ss build/vless-hy2-direct build/vless-hy2-ss; do
        printf '%-24s ' "$b"; ./$b capabilities | grep build_id
    done
}

case "$TARGET" in
    main) build_main ;;
    matrix) build_matrix ;;
    all) build_main; build_matrix ;;
    *) echo "用法: $0 [main|matrix|all]"; exit 2 ;;
esac
