@echo off
setlocal enabledelayedexpansion

:: 检查管理员权限
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] 请以管理员身份运行此批处理脚本！
    echo 正在尝试请求管理员权限...
    powershell -Command "Start-Process '%~f0' -Verb RunAs"
    exit /b
)

set "TARGET_DIR=%ProgramFiles%\AkiraProgram\PrismGoPanel"
set "SERVER_URL=%~1"
set "ENROLL_TOKEN=%~2"

echo ========================================================
echo         Prism Agent - Windows 服务安装向导
echo ========================================================
echo.

if "%SERVER_URL%"=="" (
    set /p "SERVER_URL=请输入 Prism 服务端地址 [默认 http://172.18.11.236:21114]: "
    if "!SERVER_URL!"=="" set "SERVER_URL=http://172.18.11.236:21114"
)

if "%ENROLL_TOKEN%"=="" (
    set /p "ENROLL_TOKEN=请输入终端注册码 (Enroll Token): "
)

echo.
echo 目标安装目录: %TARGET_DIR%
echo 服务端地址  : !SERVER_URL!
echo 注册码凭证  : !ENROLL_TOKEN!
echo.

:: 1. 停止旧服务
sc query PrismAgent >nul 2>&1
if %errorlevel% equ 0 (
    echo [*] 检测到已有 PrismAgent 服务，正在停止并注销...
    net stop PrismAgent >nul 2>&1
    sc delete PrismAgent >nul 2>&1
    timeout /t 2 /nobreak >nul
)

:: 2. 创建目录并复制文件
if not exist "%TARGET_DIR%" mkdir "%TARGET_DIR%"
copy /y "%~dp0prism-agent.exe" "%TARGET_DIR%\prism-agent.exe" >nul
if %errorlevel% neq 0 (
    echo [错误] 复制 prism-agent.exe 失败！
    pause
    exit /b 1
)

:: 3. 注册 Windows 服务
echo [*] 正在注册 Windows 系统服务 PrismAgent...
set "BIN_PATH=\"%TARGET_DIR%\prism-agent.exe\" --server !SERVER_URL! --token !ENROLL_TOKEN!"
sc create PrismAgent binPath= "%BIN_PATH%" start= auto DisplayName= "Prism Agent Service" >nul
if %errorlevel% neq 0 (
    echo [错误] 注册系统服务失败！
    pause
    exit /b 1
)

sc description PrismAgent "Prism Agent 终端协同与状态监控系统服务" >nul

:: 4. 启动服务
echo [*] 正在启动 PrismAgent 服务...
net start PrismAgent
if %errorlevel% neq 0 (
    echo [警告] 服务启动遇到异常，请检查本地网络或安全拦截。
) else (
    echo.
    echo [√] Prism Agent 服务安装并启动成功！
    echo     配置文件与运行时状态已存放于:
    echo     %LocalAppData%\AkiraProgram\PrismGoPanel
)

echo.
echo ========================================================
pause
