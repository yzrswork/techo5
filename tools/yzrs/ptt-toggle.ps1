# Desktop toggle for the PTT client: shows whether it is running and connected
# to the Show, then offers to stop or start it. Messages stay ASCII so Windows
# PowerShell 5.1 reads this file correctly without a BOM.
param(
    [string]$Device = '192.168.0.222',
    [int]$Port = 17327
)
$ErrorActionPreference = 'Stop'
$shell = New-Object -ComObject WScript.Shell
$title = 'YZRS PTT'
$bridge = Join-Path $PSScriptRoot 'voice_bridge.py'

function Get-Clients {
    @(Get-CimInstance Win32_Process -Filter "Name = 'python.exe' OR Name = 'pythonw.exe'" |
        Where-Object { $_.CommandLine -and $_.CommandLine.Contains($bridge) })
}
function Test-Connected {
    [bool](Get-NetTCPConnection -RemoteAddress $Device -RemotePort $Port -State Established -ErrorAction SilentlyContinue)
}

# Popup buttons: 4 = Yes/No, 0 = OK. Icons: 32 = question, 64 = info, 48 = warning. Yes = 6.
if ((Get-Clients).Count -gt 0) {
    $state = if (Test-Connected) { 'RUNNING - connected to Echo Show.' } else { 'RUNNING - waiting for Echo Show.' }
    if ($shell.Popup("PTT client is $state`n`nStop it?", 0, $title, 4 + 32) -ne 6) { exit 0 }
    & (Join-Path $PSScriptRoot 'stop-voice.ps1') | Out-Null
    Start-Sleep -Seconds 2
    if ((Get-Clients).Count -eq 0) {
        $shell.Popup('PTT client is STOPPED.', 0, $title, 64) | Out-Null
    } else {
        $shell.Popup('PTT client did not stop. Use Task Manager.', 0, $title, 48) | Out-Null
    }
    exit 0
}

if ($shell.Popup("PTT client is STOPPED.`n`nStart it?", 0, $title, 4 + 32) -ne 6) { exit 0 }
# Start exactly as Windows sign-in does, so the same private config and collector are used.
$startup = Join-Path ([Environment]::GetFolderPath('Startup')) 'YZRS TECHO5 PTT.lnk'
if (Test-Path -LiteralPath $startup) {
    Start-Process -FilePath $startup
} else {
    Start-Process -FilePath 'powershell.exe' -WindowStyle Hidden -ArgumentList @(
        '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + (Join-Path $PSScriptRoot 'start-voice.ps1') + '"'))
}
# The Whisper model loads before the client connects; allow up to a minute.
for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Seconds 2
    if (Test-Connected) {
        $shell.Popup('PTT client is RUNNING - connected to Echo Show.', 0, $title, 64) | Out-Null
        exit 0
    }
}
if ((Get-Clients).Count -gt 0) {
    $shell.Popup("PTT client is RUNNING but not connected to Echo Show.`nCheck that the Show is on and the PC uses wired LAN.", 0, $title, 48) | Out-Null
} else {
    $shell.Popup('PTT client failed to start. See ptt-*.log in the private folder.', 0, $title, 48) | Out-Null
}
