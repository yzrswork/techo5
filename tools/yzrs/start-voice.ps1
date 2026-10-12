param(
    [switch]$CheckStartup,
    [string]$PrivateDir = (Join-Path $env:LOCALAPPDATA 'YZRS-TECHO5\permanent'),
    [string]$CollectorPowerShell
)
$ErrorActionPreference = 'Stop'
if ($CollectorPowerShell) {
    $CollectorPowerShell = [IO.Path]::GetFullPath($CollectorPowerShell)
    if (-not (Test-Path -LiteralPath $CollectorPowerShell -PathType Leaf)) { throw 'Dashboard collector PowerShell unavailable' }
    # Explicit path survives a fresh login whose PATH has no PowerShell 7 entry.
    $env:YZRS_DASHBOARD_POWERSHELL = $CollectorPowerShell
}
$privateRoot = Join-Path $env:LOCALAPPDATA 'YZRS-TECHO5'
$python = Join-Path $privateRoot 'venv\Scripts\python.exe'
$config = Join-Path ([IO.Path]::GetFullPath($PrivateDir)) 'pc.json'
if ($CheckStartup) {
    & $python (Join-Path $PSScriptRoot 'voice_bridge.py') --config $config --check-startup
    if ($LASTEXITCODE -ne 0) { throw 'PTT startup check failed' }
} else {
    # Launcher is explicit; no startup registration or changes to another voice client.
    & (Join-Path $PSScriptRoot 'ensure-voice.ps1') -Python $python -Config $config
}
