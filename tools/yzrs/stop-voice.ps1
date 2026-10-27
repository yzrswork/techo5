# Stops the PTT client started from this folder. The device releases the
# microphone itself when the heartbeat stops, so a hard stop is safe.
$ErrorActionPreference = 'Stop'
$bridge = Join-Path $PSScriptRoot 'voice_bridge.py'
$clients = @(Get-CimInstance Win32_Process -Filter "Name = 'python.exe' OR Name = 'pythonw.exe'" |
    Where-Object { $_.CommandLine -and $_.CommandLine.Contains($bridge) })
if ($clients.Count -eq 0) {
    Write-Output 'PTT クライアントは起動していません。'
    exit 0
}
foreach ($client in $clients) {
    Stop-Process -Id $client.ProcessId -Force
}
Write-Output ('PTT クライアントを停止しました（' + $clients.Count + ' プロセス）。')
