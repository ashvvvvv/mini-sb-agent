#!/bin/bash
# 标准测试：vet + 全量测试 + 矩阵编译检查
# 非 Linux 环境自动进 Docker（golang:1.25.0-alpine，与生产 Go 版本一致）执行，
# 保证 /proc 等 Linux 特有用例真实运行。RUN_NATIVE=1 可强制本机执行。
set -euo pipefail
cd "$(dirname "$0")/.."

# Linux 生产机的 Go 不在默认 PATH 时自愈
if [ -x /usr/local/go/bin/go ] && ! command -v go >/dev/null 2>&1; then
    PATH=/usr/local/go/bin:$PATH
fi

if [ "$(uname -s)" != "Linux" ] && [ "${RUN_NATIVE:-0}" != "1" ]; then
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
        echo "==> 非 Linux 环境，进入 Docker（golang:1.25-alpine）执行测试"
        # MSYS(Git Bash) 会把 /src 等路径转成 Windows 路径，必须禁用
        export MSYS_NO_PATHCONV=1
        exec docker run --rm \
            -v "$(pwd -W 2>/dev/null || pwd):/src" \
            -v mini-sb-gomod:/go/pkg/mod \
            -v mini-sb-gobuild:/root/.cache/go-build \
            -w /src -e CGO_ENABLED=0 \
            golang:1.25-alpine sh scripts/test.sh
    fi
    echo "警告：docker 不可用，退回本机执行（Linux 特有用例将跳过）"
fi

echo "==> go vet"
go vet ./...
echo "==> go test"
go test ./...
echo "==> 矩阵编译检查"
go build -tags 'minimal with_vless' -o /dev/null ./cmd/mini-sb-agent
go build -tags 'minimal with_vless with_shadowsocks_outbound' -o /dev/null ./cmd/mini-sb-agent
go build -tags 'minimal with_hysteria2' -o /dev/null ./cmd/mini-sb-agent
go build -tags 'minimal with_hysteria2 with_shadowsocks_outbound' -o /dev/null ./cmd/mini-sb-agent
go build -tags 'minimal with_vless with_hysteria2' -o /dev/null ./cmd/mini-sb-agent
go build -tags 'minimal with_vless with_hysteria2 with_shadowsocks_outbound' -o /dev/null ./cmd/mini-sb-agent
go build -tags with_utls -o /dev/null ./cmd/mini-sb-agent
echo "==> 全部通过"
