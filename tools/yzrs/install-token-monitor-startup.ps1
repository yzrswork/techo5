param(
    [string]$Executable = (Join-Path $env:LOCALAPPDATA 'Programs\Token Monitor\Token Monitor.exe'),
    [string]$Settings = (Join-Path $env:APPDATA 'Token Monitor\settings.json')
)
$ErrorActionPreference = 'Stop'
if (!(Test-Path -LiteralPath $Executable) -or !(Test-Path -LiteralPath $Settings)) {
    throw 'Token Monitor installation and settings are required'
}
if (Get-Process -Name 'Token Monitor' -ErrorAction SilentlyContinue) {
    throw 'Quit Token Monitor before changing its saved preferences'
}
$saved = Get-Content -LiteralPath $Settings -Raw | ConvertFrom-Json
# Supported application preferences; never patch packaged ASAR or copy credentials.
$saved | Add-Member -NotePropertyName trayMode -NotePropertyValue $true -Force
$saved | Add-Member -NotePropertyName showTrayIcon -NotePropertyValue $true -Force
$startup = Join-Path ([Environment]::GetFolderPath('Startup')) 'Token Monitor.lnk'
$shell = New-Object -ComObject WScript.Shell
# Use one login launcher. If the app already registered its own startup, retain it.
if (!$saved.startAtLogin) {
    $link = $shell.CreateShortcut($startup)
    $link.TargetPath = [IO.Path]::GetFullPath($Executable)
    $link.WorkingDirectory = Split-Path $Executable
    $link.WindowStyle = 7
    $link.Description = 'Token Monitor supported tray mode at login'
    $link.Save()
} elseif (Test-Path -LiteralPath $startup) {
    throw 'Two login launchers exist; remove the redundant shortcut before proceeding'
}
[IO.File]::WriteAllText($Settings, ($saved | ConvertTo-Json -Depth 100), [Text.UTF8Encoding]::new($false))
$saved = $null
Write-Output 'TOKEN_MONITOR_TRAY_STARTUP=READY; existing AI sender unchanged'
