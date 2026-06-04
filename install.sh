#!/bin/sh
set -eu

APP="mini-sb-agent"
REPO="ashvvvvv/mini-sb-agent"
VERSION="v0.1.1"
INSTALL_DIR="/opt/mini-sb-agent"
RUN_DIR="/run/mini-sb-agent"
SERVICE_NAME="mini-sb-agent"
NODE_MODE="vless"
PANEL_NODE_TYPE="vless"
VLESS_NODE_ID=""
HY2_NODE_ID=""
PANEL_EVERY="60s"
NODE_RATE_MBPS="0"
HY2_UP_MBPS="0"
HY2_DOWN_MBPS="0"
HY2_IGNORE_CLIENT_BANDWIDTH="0"
GOMEMLIMIT="40MiB"
GOGC="70"
GOMAXPROCS="1"
PROTOCOL="vless"
LISTEN_ADDR="::"
VLESS_PORT=""
HY2_PORT=""
REALITY_PRIVATE_KEY=""
REALITY_SERVER_NAME="www.microsoft.com"
REALITY_SHORT_ID=""
HY2_SERVER_NAME="bing.com"
CONFIG_URL=""
CONFIG_FILE=""
PANEL_URL=""
PANEL_TOKEN=""
PANEL_NODE_ID=""
START_SERVICE="1"
FORCE="0"
ASSUME_YES="0"
INTERACTIVE="auto"

usage() {
  cat <<'EOF'
mini-sb-agent one-click installer

用法示例：

  # 交互式一键安装：选择 VLESS Reality / HY2 / 两种都装
  curl -fsSL https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/master/install.sh | sh

  # 如果 curl | sh 所在终端不能交互，就先下载再运行
  curl -fsSL https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/master/install.sh -o install.sh && sh install.sh

  # 非交互安装：只装 VLESS Reality
  sh install.sh \
    --panel-url https://board.example.com \
    --panel-token '节点密钥' \
    --node-mode vless \
    --vless-node-id 1 \
    --yes

  # 非交互安装：只装 HY2
  sh install.sh \
    --panel-url https://board.example.com \
    --panel-token '节点密钥' \
    --node-mode hy2 \
    --hy2-node-id 2 \
    --yes

  # 非交互安装：VLESS Reality + HY2 都装
  sh install.sh \
    --panel-url https://board.example.com \
    --panel-token '节点密钥' \
    --node-mode both \
    --vless-node-id 1 \
    --hy2-node-id 2 \
    --yes

参数：
  --panel-url URL                  Xboard/面板地址，例如 https://board.example.com
  --panel-token TOKEN              面板节点 token
  --node-mode MODE                 节点模式：vless、hy2、both
  --panel-node-id ID               兼容旧参数；单节点模式下等同对应节点 ID
  --vless-node-id ID               VLESS Reality 节点 ID
  --hy2-node-id ID                 HY2 节点 ID
  --panel-every DURATION           默认 60s

  --config-file PATH               可选：使用本地 config.json，跳过面板节点配置生成
  --config-url URL                 可选：下载 config.json，跳过面板节点配置生成

  --node-rate-mbps N               整节点共享限速；0 关闭
  --gomemlimit VALUE               默认 40MiB；极小内存可用 36MiB
  --gogc N                         默认 70；极小内存可用 60
  --gomaxprocs N                   默认 1
  --version TAG                    GitHub Release tag，默认 v0.1.1
  环境变量 MINI_SB_BASE_URL         可覆盖下载地址，测试/内网安装用
  --force                          覆盖旧安装
  --yes                            非交互确认，配合命令行参数使用
  --interactive                    强制进入问答式安装
  --non-interactive                禁止问答；缺参数直接报错
  --no-start                       只安装不启动
  -h, --help                       显示帮助

安装后只保留：
  /opt/mini-sb-agent/              程序、env、自动生成的 config.json、证书、卸载脚本、安装记录
  /etc/systemd/system/mini-sb-agent.service 或 /etc/init.d/mini-sb-agent
  /run/mini-sb-agent/              运行时 pid/socket，停止或卸载后清理
EOF
}
err() { echo "ERROR: $*" >&2; exit 1; }
info() { echo "==> $*"; }
need_root() { [ "$(id -u)" = "0" ] || err "请用 root 运行"; }

json_escape() {
  # POSIX shell safe enough for JSON strings used by generated config.json.
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

shell_quote() {
  # Quote one value for a POSIX sh assignment file.
  printf '%s' "$1" | sed "s/'/'\\''/g; 1s/^/'/; \$s/\$/'/"
}

is_tty() { [ -t 1 ] && [ -r /dev/tty ]; }

read_from_tty() {
  # Important for: curl .../install.sh | sh
  # In that mode stdin is the script itself, so prompts must read from /dev/tty.
  # Only use /dev/tty when stdout is actually a TTY; non-TTY test/automation
  # environments can have a /dev/tty node that cannot be opened.
  if [ -t 1 ]; then
    IFS= read -r ans </dev/tty || ans=""
  else
    IFS= read -r ans || ans=""
  fi
}

prompt() {
  var="$1"
  label="$2"
  default="$3"
  secret="${4:-0}"
  eval cur="\${$var:-}"
  [ -n "$cur" ] && default="$cur"
  if [ "$secret" = "1" ]; then
    printf '%s' "$label"
    [ -n "$default" ] && printf ' [默认：%s]' "$default"
    printf ': '
    if [ -t 1 ]; then
      stty -echo </dev/tty 2>/dev/null || true
    fi
    read_from_tty
    if [ -t 1 ]; then
      stty echo </dev/tty 2>/dev/null || true
    fi
    printf '\n'
  else
    printf '%s' "$label"
    [ -n "$default" ] && printf ' [默认：%s]' "$default"
    printf ': '
    read_from_tty
  fi
  [ -n "$ans" ] || ans="$default"
  # Keep the assignment safe even when values contain spaces, $, &, quotes, etc.
  eval "$var=\"\$ans\""
}

prompt_yes_no() {
  label="$1"
  default="$2"
  if [ "$ASSUME_YES" = "1" ]; then
    return 0
  fi
  printf '%s' "$label"
  case "$default" in
    y|Y) printf ' [Y/n]: ' ;;
    *) printf ' [y/N]: ' ;;
  esac
  read_from_tty
  [ -n "$ans" ] || ans="$default"
  case "$ans" in y|Y|yes|YES|Yes|是) return 0 ;; *) return 1 ;; esac
}

interactive_collect() {
  info "进入交互式安装。节点类型只有 VLESS Reality 和 HY2。"
  prompt PANEL_URL "面板地址，例如 https://board.example.com" "$PANEL_URL" 0
  prompt PANEL_TOKEN "面板节点 token/通讯密钥" "$PANEL_TOKEN" 1
  printf '选择安装模式：1=只装 VLESS Reality，2=只装 HY2，3=两种都装 [%s]: ' "$NODE_MODE"
  read_from_tty
  case "${ans:-$NODE_MODE}" in
    1|vless|VLESS|reality|Reality) NODE_MODE="vless" ;;
    2|hy2|HY2|hysteria|hysteria2) NODE_MODE="hy2" ;;
    3|both|BOTH|all|dual) NODE_MODE="both" ;;
    *) err "安装模式只能选 1/vless、2/hy2、3/both" ;;
  esac
  case "$NODE_MODE" in
    vless)
      prompt VLESS_NODE_ID "VLESS Reality 节点 ID" "${VLESS_NODE_ID:-$PANEL_NODE_ID}" 0
      PANEL_NODE_ID="$VLESS_NODE_ID"
      PANEL_NODE_TYPE="vless"
      ;;
    hy2)
      prompt HY2_NODE_ID "HY2 节点 ID" "${HY2_NODE_ID:-$PANEL_NODE_ID}" 0
      PANEL_NODE_ID="$HY2_NODE_ID"
      PANEL_NODE_TYPE="hysteria"
      ;;
    both)
      prompt VLESS_NODE_ID "VLESS Reality 节点 ID" "${VLESS_NODE_ID:-$PANEL_NODE_ID}" 0
      prompt HY2_NODE_ID "HY2 节点 ID" "$HY2_NODE_ID" 0
      PANEL_NODE_ID="$VLESS_NODE_ID"
      PANEL_NODE_TYPE="vless"
      ;;
  esac
  if [ "$FORCE" != "1" ] && [ -e "$INSTALL_DIR" ]; then
    if prompt_yes_no "$INSTALL_DIR 已存在，是否覆盖旧安装" n; then
      FORCE="1"
    fi
  fi
  if [ "$START_SERVICE" = "1" ]; then
    if ! prompt_yes_no "安装完成后立即启动/重启服务" y; then
      START_SERVICE="0"
    fi
  fi
}
while [ "$#" -gt 0 ]; do
  case "$1" in
    --panel-url) PANEL_URL="${2:-}"; shift 2 ;;
    --panel-token) PANEL_TOKEN="${2:-}"; shift 2 ;;
    --panel-node-id) PANEL_NODE_ID="${2:-}"; shift 2 ;;
    --node-mode) NODE_MODE="${2:-}"; shift 2 ;;
    --vless-node-id) VLESS_NODE_ID="${2:-}"; shift 2 ;;
    --hy2-node-id) HY2_NODE_ID="${2:-}"; shift 2 ;;
    --panel-node-type) PANEL_NODE_TYPE="${2:-}"; shift 2 ;;
    --panel-every) PANEL_EVERY="${2:-}"; shift 2 ;;
    --config-file) CONFIG_FILE="${2:-}"; shift 2 ;;
    --config-url) CONFIG_URL="${2:-}"; shift 2 ;;
    --protocol) PROTOCOL="${2:-}"; shift 2 ;;
    --listen) LISTEN_ADDR="${2:-}"; shift 2 ;;
    --vless-port) VLESS_PORT="${2:-}"; shift 2 ;;
    --reality-private-key) REALITY_PRIVATE_KEY="${2:-}"; shift 2 ;;
    --reality-server-name) REALITY_SERVER_NAME="${2:-}"; shift 2 ;;
    --reality-short-id) REALITY_SHORT_ID="${2:-}"; shift 2 ;;
    --hy2-port) HY2_PORT="${2:-}"; shift 2 ;;
    --hy2-server-name) HY2_SERVER_NAME="${2:-}"; shift 2 ;;
    --hy2-up-mbps) HY2_UP_MBPS="${2:-}"; shift 2 ;;
    --hy2-down-mbps) HY2_DOWN_MBPS="${2:-}"; shift 2 ;;
    --hy2-ignore-client-bandwidth) HY2_IGNORE_CLIENT_BANDWIDTH="1"; shift ;;
    --node-rate-mbps) NODE_RATE_MBPS="${2:-}"; shift 2 ;;
    --gomemlimit) GOMEMLIMIT="${2:-}"; shift 2 ;;
    --gogc) GOGC="${2:-}"; shift 2 ;;
    --gomaxprocs) GOMAXPROCS="${2:-}"; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --force) FORCE="1"; shift ;;
    --yes) ASSUME_YES="1"; shift ;;
    --interactive) INTERACTIVE="1"; shift ;;
    --non-interactive) INTERACTIVE="0"; shift ;;
    --no-start) START_SERVICE="0"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) err "未知参数：$1" ;;
  esac
done

need_root

if [ "$INTERACTIVE" = "1" ] || { [ "$INTERACTIVE" = "auto" ] && is_tty && { [ -z "$PANEL_URL" ] || [ -z "$PANEL_TOKEN" ] || { [ -z "$PANEL_NODE_ID" ] && [ -z "$VLESS_NODE_ID" ] && [ -z "$HY2_NODE_ID" ]; }; }; }; then
  interactive_collect
fi

[ -n "$PANEL_URL" ] || err "缺少 --panel-url；可直接运行 sh install.sh 进入交互式安装"
[ -n "$PANEL_TOKEN" ] || err "缺少 --panel-token；可直接运行 sh install.sh 进入交互式安装"
case "$NODE_MODE" in
  vless|VLESS|reality|Reality) NODE_MODE="vless" ;;
  hy2|HY2|hysteria|hysteria2) NODE_MODE="hy2" ;;
  both|BOTH|all|dual) NODE_MODE="both" ;;
  *) err "--node-mode 只能是 vless、hy2、both" ;;
esac
case "$NODE_MODE" in
  vless)
    [ -n "$VLESS_NODE_ID" ] || VLESS_NODE_ID="$PANEL_NODE_ID"
    PANEL_NODE_ID="$VLESS_NODE_ID"
    PANEL_NODE_TYPE="vless"
    [ -n "$PANEL_NODE_ID" ] || err "缺少 --vless-node-id 或 --panel-node-id"
    ;;
  hy2)
    [ -n "$HY2_NODE_ID" ] || HY2_NODE_ID="$PANEL_NODE_ID"
    PANEL_NODE_ID="$HY2_NODE_ID"
    PANEL_NODE_TYPE="hysteria"
    [ -n "$PANEL_NODE_ID" ] || err "缺少 --hy2-node-id 或 --panel-node-id"
    ;;
  both)
    [ -n "$VLESS_NODE_ID" ] || VLESS_NODE_ID="$PANEL_NODE_ID"
    PANEL_NODE_ID="$VLESS_NODE_ID"
    PANEL_NODE_TYPE="vless"
    [ -n "$VLESS_NODE_ID" ] || err "both 模式缺少 --vless-node-id"
    [ -n "$HY2_NODE_ID" ] || err "both 模式缺少 --hy2-node-id"
    ;;
esac

if [ -n "$CONFIG_FILE" ] && [ -n "$CONFIG_URL" ]; then
  err "--config-file 和 --config-url 只能二选一"
fi
case "$PANEL_NODE_TYPE" in vless|hysteria|hysteria2) ;; *) err "--panel-node-type 只能是 vless、hysteria2、hysteria" ;; esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ASSET_ARCH="amd64" ;;
  aarch64|arm64) ASSET_ARCH="arm64" ;;
  *) err "不支持的架构：$ARCH" ;;
esac
ASSET="mini-sb-agent-linux-$ASSET_ARCH"
BASE_URL="${MINI_SB_BASE_URL:-https://github.com/$REPO/releases/download/$VERSION}"
TMPDIR="$(mktemp -d /tmp/mini-sb-install.XXXXXX)"
cleanup() { rm -rf "$TMPDIR"; }
trap cleanup EXIT HUP INT TERM

fetch() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --connect-timeout 15 -o "$out" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$out" "$url"
  else
    err "缺少 curl/wget，无法下载"
  fi
}

install_pkgs_if_needed() {
  need_openssl="$1"
  missing=""
  command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || missing="$missing curl"
  command -v sha256sum >/dev/null 2>&1 || missing="$missing coreutils"
  if [ "$need_openssl" = "1" ]; then
    command -v openssl >/dev/null 2>&1 || missing="$missing openssl"
  fi
  [ -z "$missing" ] && return 0

  if command -v apk >/dev/null 2>&1; then
    apk add --no-cache ca-certificates curl openssl >/dev/null
  elif command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update >/dev/null
    apt-get install -y ca-certificates curl openssl >/dev/null
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y ca-certificates curl openssl >/dev/null
  elif command -v yum >/dev/null 2>&1; then
    yum install -y ca-certificates curl openssl >/dev/null
  else
    err "缺少依赖$missing，且找不到 apk/apt/dnf/yum 自动安装"
  fi
}

install_pkgs_if_needed 0

if [ -e "$INSTALL_DIR" ] && [ "$FORCE" != "1" ]; then
  err "$INSTALL_DIR 已存在。确认要覆盖请加 --force"
fi

info "下载 $ASSET $VERSION"
fetch "$BASE_URL/$ASSET" "$TMPDIR/$ASSET"
fetch "$BASE_URL/$ASSET.sha256" "$TMPDIR/$ASSET.sha256"
(
  cd "$TMPDIR"
  sha256sum -c "$ASSET.sha256"
)
chmod 0755 "$TMPDIR/$ASSET"

info "停止旧服务并清理本程序旧文件"
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  systemctl stop "$SERVICE_NAME" 2>/dev/null || true
  systemctl disable "$SERVICE_NAME" 2>/dev/null || true
  rm -f "/etc/systemd/system/$SERVICE_NAME.service"
  systemctl daemon-reload 2>/dev/null || true
fi
if command -v rc-service >/dev/null 2>&1; then
  rc-service "$SERVICE_NAME" stop 2>/dev/null || true
  rc-update del "$SERVICE_NAME" default 2>/dev/null || true
  rm -f "/etc/init.d/$SERVICE_NAME" "/etc/conf.d/$SERVICE_NAME"
fi
if [ -x "$INSTALL_DIR/$APP" ]; then
  pkill -f "$INSTALL_DIR/$APP" 2>/dev/null || true
fi
rm -rf "$INSTALL_DIR" "$RUN_DIR"
rm -f /tmp/mini-sb-agent.sock

info "安装到 $INSTALL_DIR"
mkdir -p "$INSTALL_DIR" "$RUN_DIR"
install -m 0755 "$TMPDIR/$ASSET" "$INSTALL_DIR/$APP"

if [ -n "$CONFIG_FILE" ]; then
  [ -f "$CONFIG_FILE" ] || err "config-file 不存在：$CONFIG_FILE"
  install -m 0600 "$CONFIG_FILE" "$INSTALL_DIR/config.json"
elif [ -n "$CONFIG_URL" ]; then
  fetch "$CONFIG_URL" "$INSTALL_DIR/config.json"
  chmod 0600 "$INSTALL_DIR/config.json"
else
  # config.json is generated just before mini-sb-agent starts. The generator
  # exits immediately, so it adds no runtime process or resident memory.
  rm -f "$INSTALL_DIR/config.json"
fi
cat > "$INSTALL_DIR/env" <<EOF
PANEL_URL=$(shell_quote "$PANEL_URL")
PANEL_TOKEN=$(shell_quote "$PANEL_TOKEN")
NODE_MODE=$(shell_quote "$NODE_MODE")
PANEL_NODE_ID=$(shell_quote "$PANEL_NODE_ID")
PANEL_NODE_TYPE=$(shell_quote "$PANEL_NODE_TYPE")
VLESS_NODE_ID=$(shell_quote "$VLESS_NODE_ID")
HY2_NODE_ID=$(shell_quote "$HY2_NODE_ID")
PANEL_EVERY=$(shell_quote "$PANEL_EVERY")
NODE_RATE_MBPS=$(shell_quote "$NODE_RATE_MBPS")
HY2_UP_MBPS=$(shell_quote "$HY2_UP_MBPS")
HY2_DOWN_MBPS=$(shell_quote "$HY2_DOWN_MBPS")
HY2_IGNORE_CLIENT_BANDWIDTH=$(shell_quote "$HY2_IGNORE_CLIENT_BANDWIDTH")
GOMAXPROCS=$(shell_quote "$GOMAXPROCS")
GOMEMLIMIT=$(shell_quote "$GOMEMLIMIT")
GOGC=$(shell_quote "$GOGC")
EOF
chmod 0600 "$INSTALL_DIR/env"

cat > "$INSTALL_DIR/generate-config.sh" <<'EOF'
#!/bin/sh
set -eu
APP="/opt/mini-sb-agent/mini-sb-agent"
CONFIG="/opt/mini-sb-agent/config.json"
. /opt/mini-sb-agent/env
[ -s "$CONFIG" ] && exit 0
set -- xboard-generate-config \
  --panel-url "$PANEL_URL" \
  --panel-token "$PANEL_TOKEN" \
  --node-mode "$NODE_MODE" \
  --out "$CONFIG"
if [ "${NODE_MODE:-vless}" = "vless" ]; then
  set -- "$@" --vless-node-id "$VLESS_NODE_ID"
elif [ "${NODE_MODE:-vless}" = "hy2" ]; then
  set -- "$@" --hy2-node-id "$HY2_NODE_ID"
else
  set -- "$@" --vless-node-id "$VLESS_NODE_ID" --hy2-node-id "$HY2_NODE_ID"
fi
"$APP" "$@"
EOF
chmod 0755 "$INSTALL_DIR/generate-config.sh"

cat > "$INSTALL_DIR/run.sh" <<'EOF'
#!/bin/sh
set -eu
APP="/opt/mini-sb-agent/mini-sb-agent"
CONFIG="/opt/mini-sb-agent/config.json"
API="unix:/run/mini-sb-agent/stats.sock"
. /opt/mini-sb-agent/env
export GOMAXPROCS GOMEMLIMIT GOGC
SYNC_NODE_TYPE="$PANEL_NODE_TYPE"
set -- \
  -config "$CONFIG" \
  -api "$API" \
  -panel-url "$PANEL_URL" \
  -panel-token "$PANEL_TOKEN" \
  -panel-node-id "$PANEL_NODE_ID" \
  -panel-node-type "$SYNC_NODE_TYPE" \
  -panel-every "$PANEL_EVERY" \
  -node-rate-mbps "$NODE_RATE_MBPS"
if [ "${NODE_MODE:-vless}" = "both" ]; then
  set -- "$@" -panel-hy2-node-id "$HY2_NODE_ID" -panel-hy2-node-type hysteria
fi
if [ "${HY2_UP_MBPS:-0}" != "0" ]; then
  set -- "$@" -hy2-up-mbps "$HY2_UP_MBPS"
fi
if [ "${HY2_DOWN_MBPS:-0}" != "0" ]; then
  set -- "$@" -hy2-down-mbps "$HY2_DOWN_MBPS"
fi
if [ "${HY2_IGNORE_CLIENT_BANDWIDTH:-0}" = "1" ]; then
  set -- "$@" -hy2-ignore-client-bandwidth
fi
exec "$APP" "$@"
EOF
chmod 0755 "$INSTALL_DIR/run.sh"

cat > "$INSTALL_DIR/install.meta" <<EOF
app=$APP
version=$VERSION
asset=$ASSET
installed_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
install_dir=$INSTALL_DIR
run_dir=$RUN_DIR
panel_url=$PANEL_URL
node_mode=$NODE_MODE
panel_node_id=$PANEL_NODE_ID
panel_node_type=$PANEL_NODE_TYPE
vless_node_id=$VLESS_NODE_ID
hy2_node_id=$HY2_NODE_ID
protocol=$PROTOCOL
EOF
chmod 0600 "$INSTALL_DIR/install.meta"

cat > "$INSTALL_DIR/uninstall.sh" <<'EOF'
#!/bin/sh
set -eu
APP="mini-sb-agent"
INSTALL_DIR="/opt/mini-sb-agent"
RUN_DIR="/run/mini-sb-agent"
SERVICE_NAME="mini-sb-agent"
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  systemctl stop "$SERVICE_NAME" 2>/dev/null || true
  systemctl disable "$SERVICE_NAME" 2>/dev/null || true
  rm -f "/etc/systemd/system/$SERVICE_NAME.service"
  systemctl daemon-reload 2>/dev/null || true
fi
if command -v rc-service >/dev/null 2>&1; then
  rc-service "$SERVICE_NAME" stop 2>/dev/null || true
  rc-update del "$SERVICE_NAME" default 2>/dev/null || true
  rm -f "/etc/init.d/$SERVICE_NAME" "/etc/conf.d/$SERVICE_NAME"
fi
if [ -x "$INSTALL_DIR/$APP" ]; then
  pkill -f "$INSTALL_DIR/$APP" 2>/dev/null || true
fi
rm -rf "$INSTALL_DIR" "$RUN_DIR"
rm -f /tmp/mini-sb-agent.sock
printf '%s\n' "mini-sb-agent 已卸载，仅移除了本安装器创建的文件。"
EOF
chmod 0755 "$INSTALL_DIR/uninstall.sh"

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  info "写入 systemd 服务"
  cat > "/etc/systemd/system/$SERVICE_NAME.service" <<EOF
[Unit]
Description=mini-sb-agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=GOMAXPROCS=$GOMAXPROCS
Environment=GOMEMLIMIT=$GOMEMLIMIT
Environment=GOGC=$GOGC
RuntimeDirectory=mini-sb-agent
ExecStartPre=/opt/mini-sb-agent/generate-config.sh
ExecStart=/opt/mini-sb-agent/run.sh
Restart=always
RestartSec=3
LimitNOFILE=1048576
WorkingDirectory=$INSTALL_DIR

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME" >/dev/null
  if [ "$START_SERVICE" = "1" ]; then
    systemctl restart "$SERVICE_NAME"
    sleep 1
    systemctl --no-pager --full status "$SERVICE_NAME" || true
  fi
elif command -v rc-service >/dev/null 2>&1; then
  info "写入 OpenRC 服务"
  cat > "/etc/conf.d/$SERVICE_NAME" <<EOF
# Runtime options live in /opt/mini-sb-agent/env and are read by run.sh.
EOF
  chmod 0600 "/etc/conf.d/$SERVICE_NAME"
  cat > "/etc/init.d/$SERVICE_NAME" <<'EOF'
#!/sbin/openrc-run
name="mini-sb-agent"
description="mini-sb-agent"
command="/opt/mini-sb-agent/run.sh"
command_background="yes"
pidfile="/run/mini-sb-agent/mini-sb-agent.pid"
output_log="/var/log/mini-sb-agent.log"
error_log="/var/log/mini-sb-agent.err"
start_pre() {
  checkpath -d -m 0755 /run/mini-sb-agent
  /opt/mini-sb-agent/generate-config.sh
}
EOF
  chmod 0755 "/etc/init.d/$SERVICE_NAME"
  rc-update add "$SERVICE_NAME" default >/dev/null
  if [ "$START_SERVICE" = "1" ]; then
    rc-service "$SERVICE_NAME" restart || true
    sleep 1
    rc-service "$SERVICE_NAME" status || true
  fi
else
  if [ "$START_SERVICE" = "0" ]; then
    info "找不到 systemd 或 OpenRC；已按 --no-start 只安装文件，未创建服务"
  else
    err "找不到 systemd 或 OpenRC，已安装文件但未创建服务"
  fi
fi

info "验证安装残留"
leftovers="$(find /tmp -maxdepth 1 -name 'mini-sb-install.*' -print 2>/dev/null | grep -v "$TMPDIR" || true)"
[ -z "$leftovers" ] || printf '%s\n' "$leftovers"

info "完成"
echo "安装目录：$INSTALL_DIR"
echo "卸载命令：$INSTALL_DIR/uninstall.sh"
echo "状态接口：curl --unix-socket $RUN_DIR/stats.sock http://x/stats"

