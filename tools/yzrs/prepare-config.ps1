param(
    [Parameter(Mandatory=$true)][string]$IdentityTool,
    [Parameter(Mandatory=$true)][string]$PrivateDir,
    [Parameter(Mandatory=$true)][string]$ShowIP,
    [Parameter(Mandatory=$true)][string]$PCIP,
    [Parameter(Mandatory=$true)][string]$DashboardURL,
    [int]$DeckPage = 0,
    [int]$DeckButton = 0
)
$ErrorActionPreference = 'Stop'
$PrivateDir = [IO.Path]::GetFullPath($PrivateDir)
if (Test-Path -LiteralPath $PrivateDir) { throw 'Use a new private directory; existing credentials are never overwritten' }
New-Item -ItemType Directory -Path $PrivateDir | Out-Null
$account = [Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls.exe $PrivateDir '/inheritance:r' '/grant:r' ($account+':(OI)(CI)F') | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Cannot protect private directory ACL' }
& $IdentityTool -out $PrivateDir -ip $ShowIP
if ($LASTEXITCODE -ne 0) { throw 'Identity generation failed' }
$readSecret = Read-Host '既存 Worker の READ token（書込 token は使用しません）' -AsSecureString
$secretPtr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($readSecret)
try {
    $readToken = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($secretPtr)
    $pttToken = [IO.File]::ReadAllText((Join-Path $PrivateDir 'ptt-token.txt'))
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
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($secretPtr)
    $readToken = $null
    $pttToken = $null
}
Write-Output '私用設定を作成しました。リポジトリへの追加・内容の表示は不要です。'
