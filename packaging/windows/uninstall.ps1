#Requires -RunAsAdministrator
<#
.SYNOPSIS
  Stop and remove the prism-go-panel Windows service.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File uninstall.ps1
  powershell -ExecutionPolicy Bypass -File uninstall.ps1 -RemoveData
#>
param(
    [string]$InstallDir = "C:\Program Files\prism-go-panel",
    [string]$DataDir    = "C:\ProgramData\prism-go-panel",
    [switch]$RemoveData
)
$ErrorActionPreference = "Stop"
$svcName = "prism-go-panel"

$svc = Get-Service -Name $svcName -ErrorAction SilentlyContinue
if ($svc) {
    if ($svc.Status -ne "Stopped") {
        & sc.exe stop $svcName | Out-Null
        Start-Sleep -Seconds 3
    }
    & sc.exe delete $svcName | Out-Null
    Write-Output "Service '$svcName' removed."
} else {
    Write-Output "Service '$svcName' not found, skipping."
}

if (Test-Path $InstallDir) {
    Remove-Item -Recurse -Force $InstallDir
    Write-Output "Removed $InstallDir"
}
if ($RemoveData -and (Test-Path $DataDir)) {
    Remove-Item -Recurse -Force $DataDir
    Write-Output "Removed $DataDir"
} else {
    Write-Output "Data kept at $DataDir (use -RemoveData to delete)."
}
