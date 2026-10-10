$ErrorActionPreference = 'Stop'
function Assert($condition, [string]$label) { if (-not $condition) { throw "FAIL: $label" }; Write-Output "PASS: $label" }
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('yzrs-cadence-fixture-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $scratch
$hostScript = Join-Path $scratch 'send-host-metrics.ps1'
$aiScript = Join-Path $scratch 'send-ai-usage.ps1'
$wrapper = Join-Path $PSScriptRoot 'send-operational-metrics.ps1'
$marker = Join-Path $scratch 'ai-ran'
[IO.File]::WriteAllText($hostScript, 'exit 1')
[IO.File]::WriteAllText($aiScript, "[IO.File]::WriteAllText('$marker', 'ok')")
& $wrapper -Base $scratch
Assert ((Test-Path -LiteralPath $marker)) 'host failure does not interrupt AI sender'
$marker2 = Join-Path $scratch 'host-ran'
[IO.File]::WriteAllText($hostScript, "[IO.File]::WriteAllText('$marker2', 'ok')")
[IO.File]::WriteAllText($aiScript, 'exit 1')
& $wrapper -Base $scratch
Assert ((Get-Content -LiteralPath $marker2) -eq 'ok') 'AI failure leaves valid host observation untouched'
$marker3 = Join-Path $scratch 'ai-after-timeout'
[IO.File]::WriteAllText($hostScript, 'Start-Sleep -Seconds 120')
[IO.File]::WriteAllText($aiScript, "[IO.File]::WriteAllText('$marker3', 'ok')")
& $wrapper -Base $scratch -HostTimeoutMs 1000
Assert ((Test-Path -LiteralPath $marker3)) 'hard host deadline still runs AI'
Assert ((Get-Content -LiteralPath (Join-Path $scratch 'send-operational-metrics.log') -Raw) -match 'send-host-metrics.ps1 timeout') 'deadline has safe diagnostic'
Write-Output 'Independent hidden cadence fixtures: PASS'
