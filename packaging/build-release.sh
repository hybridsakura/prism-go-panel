#!/usr/bin/env bash
# 构建全平台发布产物（Linux/macOS 二进制 + Windows 安装包 zip）。
# 用法: ./packaging/build-release.sh [版本号，如 v1.0.0]
set -euo pipefail

VERSION="${1:-dev}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DIST="$ROOT/dist/$VERSION" # 绝对路径：tar 子 shell 会切换目录
rm -rf "$DIST"
mkdir -p "$DIST"

LDFLAGS="-s -w"

build() { # GOOS GOARCH 输出名
    echo "==> $1/$2"
    CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath -ldflags="$LDFLAGS" -o "$3" ./cmd/server
}

build linux   amd64 "$DIST/rustdesk-api-server-linux-amd64"
build linux   arm64 "$DIST/rustdesk-api-server-linux-arm64"
build darwin  amd64 "$DIST/rustdesk-api-server-darwin-amd64"
build darwin  arm64 "$DIST/rustdesk-api-server-darwin-arm64"
build windows amd64 "$DIST/rustdesk-api-server-windows-amd64.exe"

# Unix 平台打 tar.gz（内含二进制 + README + 文档）
for target in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
    stage="$DIST/pkg-$target"
    mkdir -p "$stage"
    cp "$DIST/rustdesk-api-server-$target" "$stage/rustdesk-api-server"
    cp README.md "$stage/"
    cp -r docs "$stage/"
    (cd "$stage" && tar -czf "$DIST/rustdesk-api-server-$target.tar.gz" .)
    rm -rf "$stage"
done

# Windows 安装包：二进制 + 安装/卸载脚本 + 说明
WINPKG="$DIST/windows-pkg"
mkdir -p "$WINPKG"
cp "$DIST/rustdesk-api-server-windows-amd64.exe" "$WINPKG/"
cp packaging/windows/install.ps1 packaging/windows/uninstall.ps1 packaging/windows/README.txt "$WINPKG/"
if command -v zip >/dev/null 2>&1; then
    (cd "$WINPKG" && zip -q -r "$DIST/rustdesk-api-server-windows-amd64.zip" .)
else
    # Git Bash on Windows 通常没有 zip，回退到 PowerShell Compress-Archive（注意路径需转为 Windows 格式）
    WINPKG_WIN="$(cygpath -w "$WINPKG")"
    ZIP_WIN="$(cygpath -w "$DIST/rustdesk-api-server-windows-amd64.zip")"
    powershell -NoProfile -Command "Compress-Archive -Path '${WINPKG_WIN}\\*' -DestinationPath '${ZIP_WIN}' -Force"
fi
rm -rf "$WINPKG"

echo ""
echo "发布产物已生成于 $DIST:"
ls -la "$DIST"
