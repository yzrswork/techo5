param([switch]$HumanApproved)
$ErrorActionPreference = 'Stop'
$privateRoot = Join-Path $env:LOCALAPPDATA 'YZRS-TECHO5'
$manifest = Get-Content -LiteralPath (Join-Path $privateRoot 'trial-20261009\readiness.json') -Raw | ConvertFrom-Json
if (!$HumanApproved) {
    Write-Output 'HUMAN_GATE=REQUIRED; device unchanged'
    return
}
if (!(Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -eq $manifest.pc_ip -and $_.AddressState -eq 'Preferred' })) { throw 'Verified PC IP is unavailable' }
& (Join-Path $PSScriptRoot 'deploy-trial.ps1') -ShowIP $manifest.show_ip -DeviceSerial $manifest.device_serial -SSHKey (Join-Path $env:USERPROFILE '.ssh\yzrs_techo5_ed25519') -KnownHosts (Join-Path $privateRoot 'known_hosts') -Binary $manifest.binary -PrivateDir (Join-Path $privateRoot 'trial-20261009') -ExpectedSHA256 $manifest.binary_sha256 -HumanApproved
