param(
    [Parameter(Mandatory=$true)][string]$Python,
    [Parameter(Mandatory=$true)][string]$Config
)
$ErrorActionPreference = 'Stop'
$bridge = Join-Path $PSScriptRoot 'voice_bridge.py'
if (!(Test-Path -LiteralPath $Python) -or !(Test-Path -LiteralPath $Config)) { throw 'Python or private config unavailable' }
# A short lived duplicate exits at the socket lock before loading Whisper. No competing control API.
Start-Process -FilePath $Python -ArgumentList @('"'+$bridge+'"', '--config', '"'+$Config+'"') -WindowStyle Hidden
