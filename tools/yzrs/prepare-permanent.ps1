param(
    [Parameter(Mandatory=$true)][string]$SourcePrivateDir,
    [Parameter(Mandatory=$true)][string]$PrivateDir,
    [Parameter(Mandatory=$true)][string]$ShowIP,
    [Parameter(Mandatory=$true)][string]$PCIP
)
$ErrorActionPreference = 'Stop'
$SourcePrivateDir = [IO.Path]::GetFullPath($SourcePrivateDir)
$PrivateDir = [IO.Path]::GetFullPath($PrivateDir)
if (!(Test-Path -LiteralPath (Join-Path $SourcePrivateDir 'yzrs.json')) -or
    !(Test-Path -LiteralPath (Join-Path $SourcePrivateDir 'pc.json'))) {
    throw 'Previously accepted private configuration is required'
}
if (Test-Path -LiteralPath $PrivateDir) { throw 'Permanent private directory already exists; refusing overwrite' }
$show = Get-Content -LiteralPath (Join-Path $SourcePrivateDir 'yzrs.json') -Raw | ConvertFrom-Json
$pc = Get-Content -LiteralPath (Join-Path $SourcePrivateDir 'pc.json') -Raw | ConvertFrom-Json
if (!$show.enabled -or $show.ptt_addr -ne ($ShowIP+':17327') -or $pc.url -ne ('wss://'+$ShowIP+':17327/ptt')) {
    throw 'Private identity is not for the requested device'
}
if ($PCIP -notmatch '^\d{1,3}(\.\d{1,3}){3}$') { throw 'Invalid Windows PTT client address' }
foreach ($name in @('ptt-cert.pem','ptt-key.pem')) {
    $path = Join-Path $SourcePrivateDir $name
    $minimumBytes = if ($name -eq 'ptt-cert.pem') { 256 } else { 64 }
    if (!(Test-Path -LiteralPath $path) -or (Get-Item -LiteralPath $path).Length -lt $minimumBytes) { throw 'PTT identity incomplete' }
}
New-Item -ItemType Directory -Path $PrivateDir | Out-Null
$account = [Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls.exe $PrivateDir '/inheritance:r' '/grant:r' ($account+':(OI)(CI)F') | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Cannot protect permanent private directory ACL' }
try {
    Copy-Item -LiteralPath (Join-Path $SourcePrivateDir 'ptt-cert.pem') -Destination $PrivateDir
    Copy-Item -LiteralPath (Join-Path $SourcePrivateDir 'ptt-key.pem') -Destination $PrivateDir
    $show.tls_cert = '/data/misc/techo5/yzrs-private/ptt-cert.pem'
    $show.tls_key = '/data/misc/techo5/yzrs-private/ptt-key.pem'
    $show.pc_ip = $PCIP
    $pc.ca_file = Join-Path $PrivateDir 'ptt-cert.pem'
    $utf8 = New-Object Text.UTF8Encoding($false)
    [IO.File]::WriteAllText((Join-Path $PrivateDir 'yzrs.json'), ($show | ConvertTo-Json), $utf8)
    [IO.File]::WriteAllText((Join-Path $PrivateDir 'pc.json'), ($pc | ConvertTo-Json), $utf8)
    foreach ($name in @('pc.json','yzrs.json','ptt-key.pem','ptt-cert.pem')) {
        if (!(Test-Path -LiteralPath (Join-Path $PrivateDir $name))) { throw 'Permanent configuration incomplete' }
    }
} catch {
    Remove-Item -LiteralPath $PrivateDir -Recurse -Force
    throw
}
$show = $null
$pc = $null
Write-Output 'PERMANENT_PTT_CONFIG=READY; ACL=current user only; secrets not displayed'
