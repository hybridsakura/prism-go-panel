@echo off
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] 请以管理员身份运行此脚本！
    powershell -Command "Start-Process '%~f0' -Verb RunAs"
    exit /b
)

echo ========================================================
echo         Prism Agent - 卸载脚本
echo ========================================================
echo.

echo [*] 正在停止并移除 PrismAgent 服务...
net stop PrismAgent >nul 2>&1
sc delete PrismAgent >nul 2>&1

echo [*] 正在移除安装程序文件...
set "TARGET_DIR=%ProgramFiles%\AkiraProgram\PrismGoPanel"
if exist "%TARGET_DIR%\prism-agent.exe" del /f /q "%TARGET_DIR%\prism-agent.exe"
rd "%TARGET_DIR%" >nul 2>&1

echo.
echo [√] Prism Agent 服务与程序已成功移除！
echo （注意：个人配置目录 %LocalAppData%\AkiraProgram\PrismGoPanel 如需清除可手工删除）
echo.
pause
