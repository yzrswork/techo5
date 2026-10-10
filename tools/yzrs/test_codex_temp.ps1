$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'codex-temp.ps1')
function Assert($condition, [string]$label) { if (-not $condition) { throw "FAIL: $label" }; Write-Output "PASS: $label" }
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('yzrs-host-fixture-' + [guid]::NewGuid().ToString('N'))
# Isolated fixtures are retained for diagnosis; never manipulate live codex-* files.
$null = New-Item -ItemType Directory -Path $scratch
$empty = Get-CodexTempMeasurement -Root $scratch
Assert ($empty.codexTemp.bytes -eq 0 -and $empty.codexTemp.status -eq 'no-matches') 'no matching directories is valid zero'
$dir = Join-Path $scratch 'codex-fixture'
$nested = Join-Path $dir 'nested'
$null = New-Item -ItemType Directory -Path $nested
$zero = Get-CodexTempMeasurement -Root $scratch
Assert ($zero.codexTemp.bytes -eq 0 -and $zero.codexTemp.status -eq 'ok') 'empty matching directory is valid zero'
[IO.File]::WriteAllBytes((Join-Path $dir 'one.bin'), [byte[]]::new(1234))
[IO.File]::WriteAllBytes((Join-Path $nested 'two.bin'), [byte[]]::new(5678))
[IO.File]::WriteAllBytes((Join-Path $scratch 'codex-not-a-directory'), [byte[]]::new(99))
$other = Join-Path $scratch 'other'
$null = New-Item -ItemType Directory -Path $other
[IO.File]::WriteAllBytes((Join-Path $other 'ignored.bin'), [byte[]]::new(9999))
$value = Get-CodexTempMeasurement -Root $scratch
Assert ($value.codexTemp.bytes -eq 6912 -and $value.codexTemp.status -eq 'ok') 'exact recursive sum, unrelated directories and root files excluded'
$partial = Get-CodexTempMeasurement -Root $scratch -MaxEntries 1
Assert ($null -eq $partial.codexTemp.bytes -and $partial.codexTemp.status -eq 'partial') 'bounded traversal never exposes partial sum'
$missing = Get-CodexTempMeasurement -Root (Join-Path $scratch 'missing')
Assert ($null -eq $missing.codexTemp.bytes -and $missing.codexTemp.status -eq 'error') 'missing Temp root unavailable'
$link = Join-Path $nested 'outside'
$null = New-Item -ItemType Junction -Path $link -Target $other
$reparse = Get-CodexTempMeasurement -Root $scratch
Assert ($null -eq $reparse.codexTemp.bytes -and $reparse.codexTemp.status -eq 'partial') 'junction excluded and incomplete result flagged'
$rootLink = Get-CodexTempMeasurement -Root $link
Assert ($null -eq $rootLink.codexTemp.bytes -and $rootLink.codexTemp.status -eq 'error') 'reparse root refused'
# ACL failure uses a separate fixture and is restored even on failure.
$deniedRoot = Join-Path $scratch 'denied-root'
$denied = Join-Path $deniedRoot 'codex-denied'
$null = New-Item -ItemType Directory -Path $denied
$acl = Get-Acl -LiteralPath $denied
$saved = $acl.GetSecurityDescriptorSddlForm([Security.AccessControl.AccessControlSections]::All)
$sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
try {
    $rule = [Security.AccessControl.FileSystemAccessRule]::new($sid, [Security.AccessControl.FileSystemRights]::ReadAndExecute, [Security.AccessControl.AccessControlType]::Deny)
    $acl.AddAccessRule($rule)
    Set-Acl -LiteralPath $denied -AclObject $acl
    $result = Get-CodexTempMeasurement -Root $deniedRoot
    Assert ($null -eq $result.codexTemp.bytes -and $result.codexTemp.status -eq 'access-denied') 'access denied explicit, no authoritative partial total'
} finally {
    $acl.SetSecurityDescriptorSddlForm($saved)
    Set-Acl -LiteralPath $denied -AclObject $acl
}
$timeoutRoot = Join-Path $scratch 'timeout-root'
$timeoutDir = Join-Path $timeoutRoot 'codex-many'
$null = New-Item -ItemType Directory -Path $timeoutDir
for ($i=0; $i -lt 1500; $i++) { [IO.File]::WriteAllBytes((Join-Path $timeoutDir "$i.bin"), [byte[]]::new(1)) }
$timeout = Get-CodexTempMeasurement -Root $timeoutRoot -TimeoutMs 1
Assert ($null -eq $timeout.codexTemp.bytes -and $timeout.codexTemp.status -eq 'timeout') 'scan timeout explicit'
# Exercise the exact exception classification used when enumerated files vanish.
$type = [Yzrs.CodexTempCollector]
$failure = $type.GetMethod('Failure', [Reflection.BindingFlags]'NonPublic,Static')
$vanished = [Yzrs.TempResult]::new()
$failure.Invoke($null, @($vanished, [IO.FileNotFoundException]::new()))
Assert ($vanished.Status -eq 'partial' -and $vanished.Disappeared -eq 1 -and $null -eq $vanished.Bytes) 'disappearing file classified as incomplete'
Write-Output 'Windows collector fixtures: PASS'
