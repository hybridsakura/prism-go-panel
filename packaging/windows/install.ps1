#Requires -RunAsAdministrator
<#
.SYNOPSIS
  Install prism-go-panel as a Windows service.

.DESCRIPTION
  Copies the binary to the install directory, registers a Windows service
  (auto-start), passes configuration through per-service environment
  variables in the registry, and starts the service.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File install.ps1
  powershell -ExecutionPolicy Bypass -File install.ps1 -Port 21114 -AdminUser admin -AdminPass 'StrongPass!2026'
#>
param(
    [string]$InstallDir = "C:\Program Files\prism-go-panel",
    [string]$DataDir    = "C:\ProgramData\prism-go-panel",
    [int]$Port          = 21114,
    [string]$AdminUser  = "",
    [string]$AdminPass  = ""
)
$ErrorActionPreference = "Stop"
$svcName = "prism-go-panel"

$srcExe = Join-Path $PSScriptRoot "prism-go-panel-windows-amd64.exe"
if (-not (Test-Path $srcExe)) { throw "Binary not found next to this script: $srcExe" }

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir    | Out-Null

$dstExe = Join-Path $InstallDir "prism-go-panel.exe"
Copy-Item $srcExe $dstExe -Force

# Replace an existing installation cleanly.
$existing = Get-Service -Name $svcName -ErrorAction SilentlyContinue
if ($existing) {
    if ($existing.Status -ne "Stopped") { & sc.exe stop $svcName | Out-Null; Start-Sleep -Seconds 2 }
    & sc.exe delete $svcName | Out-Null
    Start-Sleep -Seconds 1
}

& sc.exe create $svcName binPath= "`"$dstExe`"" start= auto DisplayName= "RustDesk API Server (Go)" | Out-Null
if ($LASTEXITCODE -ne 0) { throw "sc.exe create failed with code $LASTEXITCODE" }
& sc.exe description $svcName "RustDesk API server: accounts, address book, heartbeat, device assets, admin console. Port $Port." | Out-Null

# Per-service environment variables: the SCM passes these to the process.
$svcEnv = @(
    "HOST=0.0.0.0",
    "PORT=$Port",
    "DB_URL=$DataDir\prism.db",
    "LOG_FILE=$DataDir\prism-go-panel.log",
    "ALLOW_REGISTRATION=true"
)
if ($AdminUser -ne "" -and $AdminPass -ne "") {
    $svcEnv += "ADMIN_USER=$AdminUser"
    $svcEnv += "ADMIN_PASS=$AdminPass"
}
Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\$svcName" -Name Environment -Value $svcEnv -Type MultiString

& sc.exe start $svcName | Out-Null
Start-Sleep -Seconds 2

$svc = Get-Service -Name $svcName
Write-Output ""
Write-Output "Service name : $svcName"
Write-Output "Status       : $($svc.Status)"
Write-Output "Admin console: http://<this-machine>:$Port/admin"
Write-Output "Data dir     : $DataDir"
Write-Output "Log file     : $DataDir\prism-go-panel.log"
if ($AdminUser -eq "") {
    Write-Output "No initial admin was preset. Register the first account at /admin - it becomes the administrator."
}
