param(
    [string]$IdentityTool,
    [Parameter(Mandatory=$true)][string]$PrivateDir,
    [Parameter(Mandatory=$true)][string]$ShowIP,
    [Parameter(Mandatory=$true)][string]$PCIP,
    [string]$DashboardURL,
    [switch]$PTTOnly,
    [switch]$ReuseIdentity,
    [int]$DeckPage = 0,
    [int]$DeckButton = 0
)
$ErrorActionPreference = 'Stop'
$PrivateDir = [IO.Path]::GetFullPath($PrivateDir)
if (Test-Path -LiteralPath $PrivateDir) {
    if (!$ReuseIdentity) { throw 'Use a new private directory or explicitly reuse an incomplete identity' }
} else {
    if ($ReuseIdentity) { throw 'Existing identity directory required' }
    New-Item -ItemType Directory -Path $PrivateDir | Out-Null
}
foreach ($name in @('pc.json','yzrs.json')) {
    if (Test-Path -LiteralPath (Join-Path $PrivateDir $name)) { throw 'Existing configuration is never overwritten' }
}
$account = [Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls.exe $PrivateDir '/inheritance:r' '/grant:r' ($account+':(OI)(CI)F') | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Cannot protect private directory ACL' }
$secretPtr = [IntPtr]::Zero
if (!$ReuseIdentity) {
    if (!$IdentityTool -or !(Test-Path -LiteralPath $IdentityTool)) { throw 'Identity tool unavailable' }
    & $IdentityTool -out $PrivateDir -ip $ShowIP
    if ($LASTEXITCODE -ne 0) { throw 'Identity generation failed' }
}
foreach ($name in @('ptt-cert.pem','ptt-key.pem','ptt-token.txt')) {
    if (!(Test-Path -LiteralPath (Join-Path $PrivateDir $name)) -or (Get-Item -LiteralPath (Join-Path $PrivateDir $name)).Length -eq 0) { throw 'Existing identity incomplete' }
}
if ($PTTOnly) {
    # Reserved invalid domain: no real service or credential is contacted.
    $DashboardURL = 'https://dashboard.invalid/dashboard'
    $readToken = 'ptt-only-placeholder-not-a-worker-credential'
} else {
    if (!$DashboardURL -or !$DashboardURL.StartsWith('https://')) { throw 'HTTPS DashboardURL required' }
    $readSecret = Read-Host 'Worker READ token' -AsSecureString
    $secretPtr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($readSecret)
    $readToken = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($secretPtr)
}
try {
    $pttToken = [IO.File]::ReadAllText((Join-Path $PrivateDir 'ptt-token.txt'))
    if ($pttToken -notmatch '^[a-f0-9]{64}$') { throw 'Invalid PTT identity token' }
    $showConfig = [ordered]@{
        enabled = $true
        dashboard_url = $DashboardURL
        dashboard_read_token = $readToken
        ptt_addr = ($ShowIP+':17327')
        pc_ip = $PCIP
        ptt_token = $pttToken
        tls_cert = '/tmp/yzrs-private/ptt-cert.pem'
        tls_key = '/tmp/yzrs-private/ptt-key.pem'
        voice_deck_page = $DeckPage
        voice_deck_button = $DeckButton
    }
    $pcConfig = [ordered]@{
        url = ('wss://'+$ShowIP+':17327/ptt')
        token = $pttToken
        ca_file = (Join-Path $PrivateDir 'ptt-cert.pem')
    }
    $utf8 = New-Object Text.UTF8Encoding($false)
    [IO.File]::WriteAllText((Join-Path $PrivateDir 'yzrs.json'), ($showConfig | ConvertTo-Json), $utf8)
    [IO.File]::WriteAllText((Join-Path $PrivateDir 'pc.json'), ($pcConfig | ConvertTo-Json), $utf8)
} finally {
    if ($secretPtr -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($secretPtr) }
    $readToken = $null
    $pttToken = $null
}
foreach ($name in @('pc.json','yzrs.json')) {
    $parsed = [IO.File]::ReadAllText((Join-Path $PrivateDir $name)) | ConvertFrom-Json
    if (!$parsed) { throw 'Configuration validation failed' }
}
$parsed = $null
Write-Output 'CONFIG=READY'
