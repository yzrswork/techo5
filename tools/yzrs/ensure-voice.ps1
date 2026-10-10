param(
    [Parameter(Mandatory=$true)][string]$Python,
    [Parameter(Mandatory=$true)][string]$Config
)
$ErrorActionPreference = 'Stop'
$bridge = Join-Path $PSScriptRoot 'voice_bridge.py'
if (!(Test-Path -LiteralPath $Python) -or !(Test-Path -LiteralPath $Config)) { throw 'Python or private config unavailable' }
# Keep credential-free startup diagnostics in the already protected private directory.
$logID = [Guid]::NewGuid().ToString('N')
$logDir = Split-Path -Parent $Config
# A short lived duplicate exits at the socket lock before loading Whisper. No competing control API.
Start-Process -FilePath $Python -ArgumentList @(('"'+$bridge+'"'), '--config', ('"'+$Config+'"')) -WindowStyle Hidden -RedirectStandardOutput (Join-Path $logDir ('ptt-'+$logID+'.log')) -RedirectStandardError (Join-Path $logDir ('ptt-'+$logID+'-error.log'))
