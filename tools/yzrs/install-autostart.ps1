param([string]$PrivateDir = (Join-Path $env:LOCALAPPDATA 'YZRS-TECHO5\permanent'),
      [string]$CollectorPowerShell)
$ErrorActionPreference = 'Stop'
$PrivateDir = [IO.Path]::GetFullPath($PrivateDir)
$config = Join-Path $PrivateDir 'pc.json'
if (!(Test-Path -LiteralPath $config)) { throw 'Permanent PTT configuration not found' }
if (-not $CollectorPowerShell) {
    $command = Get-Command pwsh -CommandType Application -ErrorAction SilentlyContinue
    if ($command) { $CollectorPowerShell = $command.Source }
}
if (-not $CollectorPowerShell) { throw 'Dashboard collector PowerShell unavailable; specify CollectorPowerShell' }
$CollectorPowerShell = [IO.Path]::GetFullPath($CollectorPowerShell)
if (-not (Test-Path -LiteralPath $CollectorPowerShell -PathType Leaf) -or $CollectorPowerShell.Contains('"')) { throw 'Dashboard collector PowerShell unavailable' }
$startup = [Environment]::GetFolderPath('Startup')
$linkPath = Join-Path $startup 'YZRS TECHO5 PTT.lnk'
# The installer may run under pwsh, whose directory contains no powershell.exe.
# Use the stable Windows inbox launcher for this PowerShell 5.1-compatible script.
$powerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
if (!(Test-Path -LiteralPath $powerShell)) { throw 'Windows PowerShell launcher unavailable' }
$launcher = Join-Path $PSScriptRoot 'start-voice.ps1'
$shell = New-Object -ComObject WScript.Shell
$link = $shell.CreateShortcut($linkPath)
$link.TargetPath = $powerShell
$link.Arguments = '-NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File "'+$launcher+'" -PrivateDir "'+$PrivateDir+'" -CollectorPowerShell "'+$CollectorPowerShell+'"'
$link.WorkingDirectory = $PSScriptRoot
$link.WindowStyle = 7
$link.Description = 'YZRS TECHO5 F8 Japanese PTT'
$link.Save()
$check = $shell.CreateShortcut($linkPath)
if ($check.TargetPath -ne $powerShell -or $check.Arguments -ne $link.Arguments) { throw 'Windows startup shortcut verification failed' }
Write-Output 'WINDOWS_PTT_STARTUP=READY; single-instance guard remains active'
