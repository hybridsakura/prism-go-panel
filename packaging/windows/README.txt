rustdesk-api-server for Windows
================================

Contents
--------
  rustdesk-api-server-windows-amd64.exe   the server binary
  install.ps1                             install as a Windows service
  uninstall.ps1                           remove the service

Install (run PowerShell as Administrator)
-----------------------------------------
  powershell -ExecutionPolicy Bypass -File install.ps1

  Optional parameters:
    -Port 21114                 listen port (default 21114, same as the original project)
    -AdminUser admin -AdminPass 'xxx'   preset the initial administrator
    -InstallDir / -DataDir      override default install/data locations

After installation the service starts automatically at boot.
Admin console: http://<this-machine>:21114/admin

Uninstall
---------
  powershell -ExecutionPolicy Bypass -File uninstall.ps1
  (database and logs are kept; add -RemoveData to delete them too)

Notes
-----
- The service stores its SQLite database and log under
  C:\ProgramData\rustdesk-api by default.
- Configuration is passed via per-service environment variables
  (registry: HKLM\SYSTEM\CurrentControlSet\Services\rustdesk-api\Environment).
- You can also run the exe directly in a console for a quick try.
