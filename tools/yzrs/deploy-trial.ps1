param(
    [Parameter(Mandatory=$true)][string]$ShowIP,
    [Parameter(Mandatory=$true)][string]$SSHKey,
    [Parameter(Mandatory=$true)][string]$Binary,
    [Parameter(Mandatory=$true)][string]$PrivateDir,
    [Parameter(Mandatory=$true)][string]$ExpectedSHA256,
    [switch]$HumanApproved
)
$ErrorActionPreference = 'Stop'
if (!$HumanApproved) { throw '実機試験は所有者の承認後に -HumanApproved を付けて実行してください。' }
if ($ShowIP -notmatch '^\d{1,3}(\.\d{1,3}){3}$' -or $ExpectedSHA256 -notmatch '^[a-f0-9]{64}$') { throw 'Invalid IP or SHA256' }
$hash = (Get-FileHash -LiteralPath $Binary -Algorithm SHA256).Hash.ToLowerInvariant()
if ($hash -ne $ExpectedSHA256) { throw 'Binary checksum mismatch' }
foreach ($name in @('yzrs.json','ptt-key.pem','ptt-cert.pem')) {
    if (!(Test-Path -LiteralPath (Join-Path $PrivateDir $name))) { throw 'Private configuration incomplete' }
}
$remote = ('root@'+$ShowIP)
# Default host-key verification is preserved. Never use StrictHostKeyChecking=no.
& ssh.exe -i $SSHKey $remote 'test ! -e /tmp/yzrs-trial && mkdir -m 700 -p /tmp/yzrs-private'
if ($LASTEXITCODE -ne 0) { throw 'SSH prerequisite failed; device not modified' }
& scp.exe -i $SSHKey $Binary ($remote+':/tmp/yzrs-'+$hash+'.elf')
if ($LASTEXITCODE -ne 0) { throw 'Binary upload failed' }
foreach ($name in @('yzrs.json','ptt-key.pem','ptt-cert.pem')) {
    & scp.exe -i $SSHKey (Join-Path $PrivateDir $name) ($remote+':/tmp/yzrs-private/'+$name)
    if ($LASTEXITCODE -ne 0) { throw 'Private configuration upload failed' }
}
& scp.exe -i $SSHKey (Join-Path $PSScriptRoot 'trial-bind.sh') ($remote+':/tmp/yzrs-private/trial-bind.sh')
if ($LASTEXITCODE -ne 0) { throw 'Trial script upload failed' }
& ssh.exe -i $SSHKey $remote ('chmod 700 /tmp/yzrs-'+$hash+'.elf; chmod 600 /tmp/yzrs-private/*; sh /tmp/yzrs-private/trial-bind.sh /tmp/yzrs-'+$hash+'.elf '+$hash+' /tmp/yzrs-private/yzrs.json')
if ($LASTEXITCODE -ne 0) { throw 'Device gate or trial start failed; inspect watchdog status' }
