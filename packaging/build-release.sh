#!/usr/bin/env bash
# 构建全平台发布产物（服务端各平台产物 + 终端 Agent 的多种形态：EXE安装版、便携版）。
# 用法: ./packaging/build-release.sh [版本号，如 v1.0.0]
set -euo pipefail

VERSION="${1:-dev}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DIST="$ROOT/dist/$VERSION" # 绝对路径：tar 子 shell 会切换目录
rm -rf "$DIST"
mkdir -p "$DIST"

LDFLAGS="-s -w"

build_server() { # GOOS GOARCH 输出名
    echo "==> server $1/$2"
    CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath -ldflags="$LDFLAGS" -o "$3" ./cmd/server
}

build_server linux   amd64 "$DIST/prism-go-panel-linux-amd64"
build_server linux   arm64 "$DIST/prism-go-panel-linux-arm64"
build_server darwin  amd64 "$DIST/prism-go-panel-darwin-amd64"
build_server darwin  arm64 "$DIST/prism-go-panel-darwin-arm64"
build_server windows amd64 "$DIST/prism-go-panel-windows-amd64.exe"

# 构建终端 Agent
echo "==> agent windows/amd64"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$LDFLAGS" -o "$DIST/prism-agent-windows-amd64.exe" ./cmd/agent

# Unix 平台打 tar.gz（内含二进制 + README + 文档）
for target in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
    stage="$DIST/pkg-$target"
    mkdir -p "$stage"
    cp "$DIST/prism-go-panel-$target" "$stage/prism-go-panel"
    cp README.md "$stage/"
    cp -r docs "$stage/"
    (cd "$stage" && tar -czf "$DIST/prism-go-panel-$target.tar.gz" .)
    rm -rf "$stage"
done

# 辅助打包函数：将目录打包为 zip
make_zip() {
    local src_dir="$1"
    local zip_file="$2"
    if command -v zip >/dev/null 2>&1; then
        (cd "$src_dir" && zip -q -r "$zip_file" .)
    else
        local src_win
        local zip_win
        src_win="$(cygpath -w "$src_dir")"
        zip_win="$(cygpath -w "$zip_file")"
        powershell -NoProfile -Command "Compress-Archive -Path '${src_win}\\*' -DestinationPath '${zip_win}' -Force"
    fi
}

# 1. 服务端 Windows 服务安装包
WINPKG="$DIST/windows-pkg"
mkdir -p "$WINPKG"
cp "$DIST/prism-go-panel-windows-amd64.exe" "$WINPKG/"
cp packaging/windows/install.ps1 packaging/windows/uninstall.ps1 packaging/windows/README.txt "$WINPKG/"
make_zip "$WINPKG" "$DIST/prism-go-panel-windows-amd64.zip"
rm -rf "$WINPKG"

# 2. Agent EXE 安装包（添加 Windows 系统服务）
AGENT_INSTALLER_PKG="$DIST/agent-installer-pkg"
mkdir -p "$AGENT_INSTALLER_PKG"
cp "$DIST/prism-agent-windows-amd64.exe" "$AGENT_INSTALLER_PKG/prism-agent.exe"
cp packaging/agent/exe-installer/install.bat "$AGENT_INSTALLER_PKG/"
cp packaging/agent/exe-installer/uninstall.bat "$AGENT_INSTALLER_PKG/"
make_zip "$AGENT_INSTALLER_PKG" "$DIST/prism-agent-windows-amd64-installer.zip"
rm -rf "$AGENT_INSTALLER_PKG"

# 3. Agent 便携启动版（不添加服务，前台运行，配置存于 %LocalAppData%\AkiraProgram\PrismGoPanel）
AGENT_PORTABLE_PKG="$DIST/agent-portable-pkg"
mkdir -p "$AGENT_PORTABLE_PKG"
cp "$DIST/prism-agent-windows-amd64.exe" "$AGENT_PORTABLE_PKG/prism-agent.exe"
cp packaging/agent/portable/start-portable.bat "$AGENT_PORTABLE_PKG/"
cp packaging/agent/portable/README.txt "$AGENT_PORTABLE_PKG/"
make_zip "$AGENT_PORTABLE_PKG" "$DIST/prism-agent-windows-amd64-portable.zip"
rm -rf "$AGENT_PORTABLE_PKG"

echo ""
echo "发布产物已生成于 $DIST:"
ls -la "$DIST"
