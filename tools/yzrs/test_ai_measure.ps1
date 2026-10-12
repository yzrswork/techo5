$ErrorActionPreference = 'Stop'
function Assert($condition, [string]$label) { if (-not $condition) { throw ('FAIL: ' + $label) }; Write-Output ('PASS: ' + $label) }
. (Join-Path $PSScriptRoot 'send-ai-usage.ps1')
$fixture = @{
    identity = 'private-fixture-never-forward'
    limits = @{ providers = @(@{
        provider = 'codex'; planLabel = 'Pro'; accountLabel = 'private-fixture-never-forward';
        status = 'ok'; stale = $false; identity = 'private-fixture-never-forward';
        windows = @(
            @{ kind = 'session'; remainingPercent = 70; resetsAt = '2026-10-12T01:00:00Z'; identity = 'private-fixture-never-forward' },
            @{ kind = 'weekly'; remainingPercent = 80; resetsAt = '2026-10-14T01:00:00Z' }
        )
    }) }
    periods = @{ today = @{ clients = @{ codex = 1234 } } }
}
$native = Get-NativeAI $fixture
$encoded = ConvertTo-Json $native -Depth 8 -Compress
Assert (($native.Keys -join ',') -eq 'source,updatedAt,codex') 'native AI fields are flattened'
Assert ($native.codex.plan -eq 'Pro' -and $native.codex.todayTokens -eq 1234) 'real counts and plan are preserved'
Assert ($encoded -notmatch 'private-fixture|identity|accountLabel|limits|periods') 'raw telemetry and identities are not forwarded'
Assert ($native.codex.session.resetsAt -eq '2026-10-12T01:00:00.000Z') 'reset timestamp normalized'
$fixture.periods.today.clients.codex = $null
$rejected = $false
try { $null = Get-NativeAI $fixture } catch { $rejected = $true }
Assert $rejected 'unknown tokens are rejected rather than zeroed'
$fixture.periods.today.clients.codex = 1234
$fixture.limits.providers[0].windows[0].remainingPercent = 101
$rejected = $false
try { $null = Get-NativeAI $fixture } catch { $rejected = $true }
Assert $rejected 'invalid limit rejected'
$fixture.limits.providers[0].windows[0].remainingPercent = 70

# Run the exact MeasureOnly path with synthetic DPAPI and a local HTTP stub.
# The real installed credentials and HTTP endpoints are never accessed.
$oldLocalAppData = $env:LOCALAPPDATA
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('yzrs-ai-fixture-' + [guid]::NewGuid().ToString('N'))
try {
    $env:LOCALAPPDATA = $scratch
    $fixtureBase = Join-Path $scratch 'YZRS\EchoDashboard'
    $null = New-Item -ItemType Directory -Path $fixtureBase -Force
    $cipher = ConvertTo-SecureString 'synthetic-hub-test-secret' -AsPlainText -Force | ConvertFrom-SecureString
    [IO.File]::WriteAllText((Join-Path $fixtureBase 'token-monitor-secret.dpapi'), $cipher)
    Assert ((Read-DpapiText (Join-Path $fixtureBase 'token-monitor-secret.dpapi')) -eq 'synthetic-hub-test-secret') 'synthetic credential fixture readable'
    $global:yzrsStatsFixture = ConvertTo-Json $fixture -Depth 10 -Compress
    $global:yzrsFixtureCalls = @()
    function Invoke-WebRequest {
        param($Uri, $Headers, $Method, $Body, $TimeoutSec, [switch]$UseBasicParsing)
        $global:yzrsFixtureCalls += [string]$Uri
        if ($Uri -eq 'http://127.0.0.1:17321/api/health') { return @{ StatusCode = 200 } }
        if ($Uri -eq 'http://127.0.0.1:17321/api/stats') { return @{ StatusCode = 200; Content = $global:yzrsStatsFixture } }
        throw 'Unexpected fixture endpoint'
    }
    $body = & (Join-Path $PSScriptRoot 'send-ai-usage.ps1') -MeasureOnly
    $result = ConvertFrom-Json $body -AsHashtable -DateKind String
    Assert ($result.codex.todayTokens -eq 1234 -and -not $result.ContainsKey('ai')) 'MeasureOnly runs without upload credential'
    Assert ($global:yzrsFixtureCalls.Count -eq 2 -and -not ($global:yzrsFixtureCalls -match 'https://')) 'MeasureOnly performs no Worker upload'
    Assert (-not (Test-Path (Join-Path $fixtureBase 'send-ai-usage.log'))) 'MeasureOnly leaves installed logs unchanged'
    Assert ($body -notmatch 'private-fixture|synthetic-hub') 'MeasureOnly output contains neither raw stats nor credential'
} finally {
    $env:LOCALAPPDATA = $oldLocalAppData
    Remove-Item Function:\Invoke-WebRequest -ErrorAction SilentlyContinue
    Remove-Variable yzrsStatsFixture,yzrsFixtureCalls -Scope Global -ErrorAction SilentlyContinue
    # Explicitly verified fixture subtree only.
    $resolved = [IO.Path]::GetFullPath($scratch)
    $temp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
    if ($resolved.StartsWith($temp, [StringComparison]::OrdinalIgnoreCase) -and (Split-Path $resolved -Leaf) -like 'yzrs-ai-fixture-*') {
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}



