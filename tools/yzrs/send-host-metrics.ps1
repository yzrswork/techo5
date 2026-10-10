param([switch]$MeasureOnly, [string]$OutputPath)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'codex-temp.ps1')
$base = Join-Path $env:LOCALAPPDATA 'YZRS\EchoDashboard'

function Write-HostLog([string]$event) {
    try { [IO.File]::AppendAllText((Join-Path $base 'send-host-metrics.log'), ([DateTimeOffset]::UtcNow.ToString('o') + ' ' + $event + [Environment]::NewLine)) } catch {}
}
$stage = 'collection'
try {
    $payload = Get-CodexTempMeasurement
    $body = ConvertTo-Json -InputObject $payload -Depth 4 -Compress
    if ($OutputPath) { [IO.File]::WriteAllText($OutputPath, $body, [Text.UTF8Encoding]::new($false)) }
    if ($MeasureOnly) { Write-Output $body; exit 0 }
    Write-HostLog ('measurement-' + $payload.codexTemp.status)
    $stage = 'credentials'
    $path = Join-Path $base 'host-write-token.dpapi'
    if (-not (Test-Path -LiteralPath $path)) { Write-HostLog 'upload-credentials-unavailable'; exit 1 }
    $secure = Get-Content -LiteralPath $path -Raw | ConvertTo-SecureString
    $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try { $token = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr) }
    finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr); $secure.Dispose() }
    try {
        $stage = 'upload'
        # Preserve the actual observation timestamp. A small fixed delivery
        # buffer lets the server clock catch up without accepting future data.
        Start-Sleep -Seconds 2
        $reply = Invoke-WebRequest -UseBasicParsing -Method Post -Uri 'https://echo-show-dashboard.yzrswork.workers.dev/dashboard/host' -Headers @{ Authorization = "Bearer $token" } -ContentType 'application/json; charset=utf-8' -Body $body -TimeoutSec 8 -MaximumRedirection 0
        $result = ConvertFrom-Json -InputObject $reply.Content
        if ($reply.StatusCode -ne 200 -or $result.ok -ne $true) { throw 'Upload failed' }
        Write-HostLog ('upload-success-' + $payload.codexTemp.status)
    } finally { $token = $null }
} catch {
    $event = $stage + '-failed-' + $_.Exception.GetType().Name
    if ($_.Exception -is [Microsoft.PowerShell.Commands.HttpResponseException]) {
        $event += '-http-' + [int]$_.Exception.Response.StatusCode
    }
    Write-HostLog $event
    exit 1
}
