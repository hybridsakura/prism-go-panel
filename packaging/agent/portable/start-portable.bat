@echo off
setlocal enabledelayedexpansion

echo ========================================================
echo       Prism Agent - 便携启动版（免安装 / 不添加系统服务）
echo ========================================================
echo.

set "CONFIG_DIR=%LocalAppData%\AkiraProgram\PrismGoPanel"
set "STATE_FILE=!CONFIG_DIR!\state.json"

if not exist "!CONFIG_DIR!" mkdir "!CONFIG_DIR!"

set "SERVER_URL=%PRISM_SERVER%"
set "ENROLL_TOKEN=%PRISM_ENROLL_TOKEN%"

if not exist "!STATE_FILE!" (
    echo [*] 首次运行需要连接服务端并完成注册。
    if "!SERVER_URL!"=="" (
        set /p "SERVER_URL=请输入 Prism 服务端地址 [默认 http://172.18.11.236:21114]: "
        if "!SERVER_URL!"=="" set "SERVER_URL=http://172.18.11.236:21114"
    )
    if "!ENROLL_TOKEN!"=="" (
        set /p "ENROLL_TOKEN=请输入终端注册码 (Enroll Token): "
    )
) else (
    echo [*] 检测到已有配置状态: !STATE_FILE!
)

echo.
echo 启动中... (配置目录: !CONFIG_DIR!)
echo [提示] 本模式为前台便携运行，关闭本窗口即停止 Agent。
echo.

"%~dp0prism-agent.exe" --server "!SERVER_URL!" --token "!ENROLL_TOKEN!" --data-dir "!CONFIG_DIR!"

pause
