# Desktop "safe to unplug" for the Show. With the AC adapter connected the Show
# powers on again by itself after a poweroff (boot reason usb), so instead of
# powering off this stops the daemon cleanly, syncs and makes /data read-only,
# then tells the owner to unplug. Plugging in again starts the Show. Cancel
# restarts the Show instead. Messages stay ASCII for Windows PowerShell 5.1.
param(
    [string]$PrivateDir = (Join-Path $env:LOCALAPPDATA 'YZRS-TECHO5\permanent'),
    [string]$Key = (Join-Path $env:USERPROFILE '.ssh\yzrs_techo5_ed25519'),
    [string]$KnownHosts = (Join-Path $env:LOCALAPPDATA 'YZRS-TECHO5\known_hosts')
)
$ErrorActionPreference = 'Stop'
$shell = New-Object -ComObject WScript.Shell
$title = 'Echo Show OFF'
# Popup buttons: 1 = OK/Cancel, 4 = Yes/No, 0 = OK. Icons: 32 question, 64 info, 48 warning.
# Results: 1 = OK, 2 = Cancel, 6 = Yes.

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

function Invoke-Show([string]$command) {
    $ssh = Join-Path $env:SystemRoot 'System32\OpenSSH\ssh.exe'
    # No double quotes in $command: Windows PowerShell 5.1 strips them from native arguments.
    $output = & $ssh -i $Key -o BatchMode=yes -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$KnownHosts" `
        -o ConnectTimeout=8 -o KexAlgorithms=curve25519-sha256 "root@$device" $command 2>$null
    return @{ Code = $LASTEXITCODE; Output = ($output -join "`n") }
}

if (-not (Test-Up)) {
    $shell.Popup("Echo Show ($device) is not reachable. It may already be off or unplugged.", 0, $title, 64) | Out-Null
    exit 0
}
if ($shell.Popup("Prepare Echo Show ($device) to be unplugged?`n`nThe dashboard and PTT stop. Then unplug the AC adapter.", 0, $title, 4 + 32) -ne 6) { exit 0 }

# The same clean stop as the Show's own shutdown.sh. /run/techo5/hold keeps init
# from starting the daemon again; it lives in /run, so a restart clears it.
$prepare = 'PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; ' +
    'touch /run/techo5/hold; ' +
    'if pidof techo5 >/dev/null; then kill -TERM $(pidof techo5); n=0; ' +
    'while [ $n -lt 10 ] && pidof techo5 >/dev/null; do sleep 1; n=$((n+1)); done; fi; ' +
    'pidof techo5 >/dev/null && echo DAEMON_RUNNING; ' +
    'cp /run/boot.log /data/techo5-linux/boot.log 2>/dev/null; sync; ' +
    'if mount -o remount,ro /data; then echo READY; else sync; echo DATA_RW; fi'
$result = Invoke-Show $prepare
if ($result.Code -ne 0 -or $result.Output -notmatch 'READY|DATA_RW') {
    $shell.Popup("Could not prepare Echo Show (ssh exit $($result.Code)). Nothing was unplugged; it may need a restart.", 0, $title, 48) | Out-Null
    exit 1
}
if ($result.Output -match 'READY' -and $result.Output -notmatch 'DAEMON_RUNNING') {
    $message = "Safe to unplug now.`n`nUnplug the AC adapter. Plug it in again to start Echo Show.`n`nCancel = restart Echo Show instead."
    $icon = 64
} else {
    $message = "Mostly safe to unplug: data was written, but /data could not be made read-only.`n`nUnplug the AC adapter, or Cancel to restart Echo Show."
    $icon = 48
}
if ($shell.Popup($message, 0, $title, 1 + $icon) -eq 2) {
    # A delayed background reboot dies with the SSH session, so reboot directly;
    # the session may then end with an error, so confirm by watching the Show go down.
    Invoke-Show 'PATH=/usr/sbin:/usr/bin:/sbin:/bin; sync; reboot' | Out-Null
    $down = $false
    for ($i = 0; $i -lt 10 -and -not $down; $i++) { Start-Sleep -Seconds 2; $down = -not (Test-Up) }
    if ($down) {
        $shell.Popup('Echo Show is restarting.', 0, $title, 64) | Out-Null
    } else {
        $shell.Popup('Could not restart Echo Show. Unplug and plug in the AC adapter.', 0, $title, 48) | Out-Null
    }
}
