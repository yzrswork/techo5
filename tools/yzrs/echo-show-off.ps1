# Desktop power-off for the Show: confirms, sends a clean "sync; poweroff" over
# strict SSH, then watches whether the Show stays off or restarts (it may restart
# while the AC adapter is connected). Messages stay ASCII for Windows PowerShell 5.1.
param(
    [string]$PrivateDir = (Join-Path $env:LOCALAPPDATA 'YZRS-TECHO5\permanent'),
    [string]$Key = (Join-Path $env:USERPROFILE '.ssh\yzrs_techo5_ed25519'),
    [string]$KnownHosts = (Join-Path $env:LOCALAPPDATA 'YZRS-TECHO5\known_hosts')
)
$ErrorActionPreference = 'Stop'
$shell = New-Object -ComObject WScript.Shell
$title = 'Echo Show OFF'
# Popup buttons: 4 = Yes/No, 0 = OK. Icons: 32 = question, 64 = info, 48 = warning. Yes = 6.

# The Show address comes from the private PTT config; never print the config itself.
try {
    $cfg = Get-Content -LiteralPath (Join-Path $PrivateDir 'pc.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    $device = ([uri]$cfg.url).Host
} catch {
    $shell.Popup('Cannot read the Show address from pc.json.', 0, $title, 48) | Out-Null
    exit 1
}

function Test-Up {
    $client = New-Object Net.Sockets.TcpClient
    try { return $client.ConnectAsync($device, 22).Wait(2000) -and $client.Connected } catch { return $false } finally { $client.Dispose() }
}

if (-not (Test-Up)) {
    $shell.Popup("Echo Show ($device) is not reachable. It may already be off.", 0, $title, 64) | Out-Null
    exit 0
}
if ($shell.Popup("Turn off Echo Show ($device)?`n`nDashboard and PTT stop until it is turned on again.", 0, $title, 4 + 32) -ne 6) { exit 0 }

$ssh = Join-Path $env:SystemRoot 'System32\OpenSSH\ssh.exe'
$arguments = @('-i', $Key, '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes', '-o', "UserKnownHostsFile=$KnownHosts",
    '-o', 'ConnectTimeout=8', '-o', 'KexAlgorithms=curve25519-sha256', "root@$device",
    # Detach so the SSH session returns before the network goes down.
    'sync; (sleep 1; /sbin/poweroff) >/dev/null 2>&1 </dev/null &')
& $ssh @arguments 2>$null
if ($LASTEXITCODE -ne 0) {
    $shell.Popup("Could not send power-off (ssh exit $LASTEXITCODE). Nothing was changed.", 0, $title, 48) | Out-Null
    exit 1
}

# Watch for up to 2 minutes: down and staying down means off; back up means it restarted.
$wentDown = $false
for ($i = 0; $i -lt 60; $i++) {
    Start-Sleep -Seconds 2
    $up = Test-Up
    if (-not $up) { $wentDown = $true }
    elseif ($wentDown) {
        $shell.Popup("Echo Show restarted instead of staying off.`nThis happens while the AC adapter is connected. Unplug it now to keep it off.", 0, $title, 48) | Out-Null
        exit 0
    }
}
if ($wentDown) {
    $shell.Popup('Echo Show is OFF.', 0, $title, 64) | Out-Null
} else {
    $shell.Popup('Echo Show did not turn off. Check it on the device.', 0, $title, 48) | Out-Null
}
