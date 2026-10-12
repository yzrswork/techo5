param([switch]$MeasureOnly)
$ErrorActionPreference = 'Stop'
$base = Join-Path $env:LOCALAPPDATA 'YZRS\EchoDashboard'
$hubFile = Join-Path $base 'token-monitor-secret.dpapi'
$aiFile = Join-Path $base 'ai-write-token.dpapi'
$logFile = Join-Path $base 'send-ai-usage.log'
$workerUrl = 'https://echo-show-dashboard.yzrswork.workers.dev/dashboard/ai'

function Write-SafeLog([string]$event) {
    if ($MeasureOnly) { return }
    $stamp = [DateTimeOffset]::Now.ToString('yyyy-MM-dd HH:mm:ss zzz', [Globalization.CultureInfo]::InvariantCulture)
    try {
        [IO.File]::AppendAllText($logFile, ($stamp + ' ' + $event + [Environment]::NewLine), [Text.UTF8Encoding]::new($false))
    } catch {}
}

function Read-DpapiText([string]$path) {
    $secure = Get-Content -Raw -LiteralPath $path | ConvertTo-SecureString
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
        $secure.Dispose()
    }
}

function Get-WindowRole($window) {
    $parts = @($window.kind, $window.limitId, $window.label, $window.resetDescription) | Where-Object { $_ -is [string] }
    $hint = $parts -join ' '
    if ($hint -match '(?i)session|5\s*hour|5h') { return 'session' }
    if ($hint -match '(?i)weekly|week|7\s*day|7d') { return 'weekly' }
    return 'other'
}

function New-UsageWindow($window) {
    $remaining = $window.remainingPercent
    if ($null -eq $remaining -or $remaining -is [bool] -or $remaining -isnot [ValueType]) { throw 'schema' }
    $percent = [double]$remaining
    if ([double]::IsNaN($percent) -or [double]::IsInfinity($percent) -or $percent -lt 0 -or $percent -gt 100) { throw 'schema' }
    $reset = $window.resetsAt
    if ($reset -isnot [string] -or $reset -notmatch '^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:[Zz]|[+-]\d{2}:\d{2})$') { throw 'schema' }
    $instant = [DateTimeOffset]::MinValue
    if (-not [DateTimeOffset]::TryParse($reset, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::RoundtripKind, [ref]$instant)) { throw 'schema' }
    return [ordered]@{
        remainingPercent = $percent
        resetsAt = $instant.ToUniversalTime().ToString("yyyy-MM-dd'T'HH:mm:ss.fff'Z'", [Globalization.CultureInfo]::InvariantCulture)
    }
}

function Get-NativeAI($stats) {
        $providers = @($stats.limits.providers | Where-Object { $_.provider -ceq 'codex' })
        if ($providers.Count -ne 1) { throw 'schema' }
        $codex = $providers[0]

        $plan = [string]$codex.planLabel
        if ([string]::IsNullOrWhiteSpace($plan)) { $plan = [string]$codex.accountLabel }
        $plan = $plan.Trim()
        if ([string]::IsNullOrWhiteSpace($plan) -or $plan -notmatch '^[\p{L}\p{N}][\p{L}\p{N} ._+-]{0,31}$') { throw 'schema' }

        if ($codex.status -isnot [string] -or [string]::IsNullOrWhiteSpace($codex.status) -or $codex.stale -isnot [bool]) { throw 'schema' }

        $rawToday = $stats.periods.today.clients.codex
        if ($null -eq $rawToday -or $rawToday -is [bool] -or $rawToday -isnot [ValueType]) { throw 'schema' }
        $today = [double]$rawToday
        if ([double]::IsNaN($today) -or [double]::IsInfinity($today) -or $today -lt 0 -or $today -gt 9007199254740991 -or [math]::Truncate($today) -ne $today) { throw 'schema' }

        $windows = @($codex.windows)
        $session = @($windows | Where-Object { (Get-WindowRole $_) -eq 'session' })
        $weekly = @($windows | Where-Object { (Get-WindowRole $_) -eq 'weekly' })
        if ($session.Count -ne 1 -or $weekly.Count -ne 1) { throw 'schema' }

        $observedAt = [DateTimeOffset]::UtcNow.ToString("yyyy-MM-dd'T'HH:mm:ss.fff'Z'", [Globalization.CultureInfo]::InvariantCulture)
        return [ordered]@{
            source = 'token-monitor'
            updatedAt = $observedAt
            codex = [ordered]@{
                    plan = $plan
                    status = [string]$codex.status
                    stale = [bool]$codex.stale
                    todayTokens = [long]$today
                    session = (New-UsageWindow $session[0])
                    weekly = (New-UsageWindow $weekly[0])
            }
        }
}

# Dot sourcing exposes only the pure allowlist extractor to offline fixture tests.
if ($MyInvocation.InvocationName -eq '.') { return }

try {
    if (-not (Test-Path -LiteralPath $hubFile) -or (-not $MeasureOnly -and -not (Test-Path -LiteralPath $aiFile))) {
        Write-SafeLog 'skip-credentials-unavailable'
        exit 0
    }

    try {
        $health = Invoke-WebRequest -UseBasicParsing -Uri 'http://127.0.0.1:17321/api/health' -TimeoutSec 5
        if ([int]$health.StatusCode -ne 200) {
            Write-SafeLog 'skip-token-monitor-offline'
            exit 0
        }
    } catch {
        Write-SafeLog 'skip-token-monitor-offline'
        exit 0
    }

    try {
        $hubSecret = Read-DpapiText $hubFile
        try {
            $raw = (Invoke-WebRequest -UseBasicParsing -Uri 'http://127.0.0.1:17321/api/stats' -Headers @{ Authorization = "Bearer $hubSecret" } -TimeoutSec 12).Content
        } finally {
            $hubSecret = $null
        }
        $stats = ConvertFrom-Json -InputObject $raw -AsHashtable -DateKind String
        $raw = $null
    } catch {
        Write-SafeLog 'skip-stats-unavailable'
        exit 0
    }

    try {
        $native = Get-NativeAI $stats
        if ($MeasureOnly) {
            Write-Output (ConvertTo-Json -InputObject $native -Depth 8 -Compress)
            exit 0
        }
        $payload = [ordered]@{ source = $native.source; updatedAt = $native.updatedAt; ai = [ordered]@{ codex = $native.codex } }
        $body = ConvertTo-Json -InputObject $payload -Depth 8 -Compress
    } catch {
        Write-SafeLog 'schema-invalid'
        exit 1
    } finally {
        $stats = $null
    }

    try {
        $aiToken = Read-DpapiText $aiFile
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Method Post -Uri $workerUrl -Headers @{ Authorization = "Bearer $aiToken" } -ContentType 'application/json; charset=utf-8' -Body $body -TimeoutSec 20
        } finally {
            $aiToken = $null
        }
        $reply = ConvertFrom-Json -InputObject $response.Content -DateKind String
        if ([int]$response.StatusCode -ne 200 -or $reply.ok -ne $true) {
            Write-SafeLog ('worker-http-' + [int]$response.StatusCode)
            exit 1
        }
        Write-SafeLog 'success'
        exit 0
    } catch {
        Write-SafeLog 'worker-request-failed'
        exit 1
    } finally {
        $body = $null
        $payload = $null
    }
} catch {
    Write-SafeLog 'sender-error'
    exit 1
}