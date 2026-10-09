param([string]$PrivateDir = (Join-Path $env:LOCALAPPDATA 'YZRS-TECHO5\permanent'))
$ErrorActionPreference = 'Stop'
$PrivateDir = [IO.Path]::GetFullPath($PrivateDir)
$config = Join-Path $PrivateDir 'pc.json'
if (!(Test-Path -LiteralPath $config)) { throw 'Permanent PTT configuration not found' }
$startup = [Environment]::GetFolderPath('Startup')
$linkPath = Join-Path $startup 'YZRS TECHO5 PTT.lnk'
$powerShell = Join-Path $PSHOME 'powershell.exe'
$launcher = Join-Path $PSScriptRoot 'start-voice.ps1'
$shell = New-Object -ComObject WScript.Shell
$link = $shell.CreateShortcut($linkPath)
$link.TargetPath = $powerShell
$link.Arguments = '-NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File "'+$launcher+'" -PrivateDir "'+$PrivateDir+'"'
$link.WorkingDirectory = $PSScriptRoot
$link.WindowStyle = 7
$link.Description = 'YZRS TECHO5 F8 Japanese PTT'
$link.Save()
$check = $shell.CreateShortcut($linkPath)
if ($check.TargetPath -ne $powerShell -or $check.Arguments -notlike '*start-voice.ps1*') { throw 'Windows startup shortcut verification failed' }
Write-Output 'WINDOWS_PTT_STARTUP=READY; single-instance guard remains active'
