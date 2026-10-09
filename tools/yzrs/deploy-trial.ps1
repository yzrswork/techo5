param(
    [Parameter(Mandatory=$true)][string]$ShowIP,
    [Parameter(Mandatory=$true)][string]$SSHKey,
    [Parameter(Mandatory=$true)][string]$Binary,
    [Parameter(Mandatory=$true)][string]$PrivateDir,
    [Parameter(Mandatory=$true)][string]$ExpectedSHA256,
    [Parameter(Mandatory=$true)][string]$KnownHosts,
    [Parameter(Mandatory=$true)][string]$DeviceSerial,
    [switch]$HumanApproved
)
$ErrorActionPreference = 'Stop'
if (!$HumanApproved) { throw 'Owner approval required: use -HumanApproved only after the Human Gate' }
if ($ShowIP -notmatch '^\d{1,3}(\.\d{1,3}){3}$' -or $ExpectedSHA256 -notmatch '^[a-f0-9]{64}$' -or $DeviceSerial -notmatch '^[A-Z0-9]+$') { throw 'Invalid IP, serial or SHA256' }
$hash = (Get-FileHash -LiteralPath $Binary -Algorithm SHA256).Hash.ToLowerInvariant()
if ($hash -ne $ExpectedSHA256) { throw 'Binary checksum mismatch' }
foreach ($name in @('yzrs.json','ptt-key.pem','ptt-cert.pem')) {
    if (!(Test-Path -LiteralPath (Join-Path $PrivateDir $name))) { throw 'Private configuration incomplete' }
}
$remote = ('root@'+$ShowIP)
# Default host-key verification is preserved. Never use StrictHostKeyChecking=no.
$sshOptions = @('-o','KexAlgorithms=curve25519-sha256','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-i',$SSHKey)
if ($KnownHosts) { $sshOptions += @('-o',('UserKnownHostsFile='+[IO.Path]::GetFullPath($KnownHosts))) }
if (!(Test-Path -LiteralPath $SSHKey) -or !(Test-Path -LiteralPath $KnownHosts)) { throw 'Verified SSH identity required' }
# All device gates precede uploads or directory creation. Repeat mount/slot checks in trial-bind.sh.
$preflight = 'set -eu; export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; grep -q androidboot.serialno=SERIAL /proc/cmdline; status=$(STORE=/store slotctl status); printf "%s\n" "$status" | grep -q "^active: a$"; printf "%s\n" "$status" | grep -q "^booted: a$"; printf "%s\n" "$status" | grep -q "^slot a: good"; printf "%s\n" "$status" | grep -q "^slot b: empty"; test ! -e /tmp/yzrs-trial; test ! -e /tmp/yzrs-private; test ! -e /data/misc/techo5/yzrs-trial-backup; if awk ''$2 == "/usr/local/bin/techo5" {found=1} END {exit !found}'' /proc/mounts; then exit 1; fi; echo DEVICE_GATE=PASS'
# Windows PowerShell 5.1 strips embedded quotes in native command arguments.
# Send this non-secret read-only script via stdin so the remote shell sees exact quoting.
($preflight.Replace('SERIAL',$DeviceSerial)) | & ssh.exe @sshOptions $remote 'tr -d ''\r'' | sh -s'
if ($LASTEXITCODE -ne 0) { throw 'SSH prerequisite failed; device not modified' }
& ssh.exe @sshOptions $remote 'umask 077; mkdir -m 700 /tmp/yzrs-private'
if ($LASTEXITCODE -ne 0) { throw 'Private staging unavailable' }
& scp.exe -O @sshOptions $Binary ($remote+':/tmp/yzrs-'+$hash+'.elf')
if ($LASTEXITCODE -ne 0) { throw 'Binary upload failed' }
foreach ($name in @('yzrs.json','ptt-key.pem','ptt-cert.pem')) {
    & scp.exe -O @sshOptions (Join-Path $PrivateDir $name) ($remote+':/tmp/yzrs-private/'+$name)
    if ($LASTEXITCODE -ne 0) { throw 'Private configuration upload failed' }
}
& scp.exe -O @sshOptions (Join-Path $PSScriptRoot 'trial-bind.sh') ($remote+':/tmp/yzrs-private/trial-bind.sh')
if ($LASTEXITCODE -ne 0) { throw 'Trial script upload failed' }
& ssh.exe @sshOptions $remote ('chmod 700 /tmp/yzrs-'+$hash+'.elf; chmod 600 /tmp/yzrs-private/*; sh /tmp/yzrs-private/trial-bind.sh /tmp/yzrs-'+$hash+'.elf '+$hash+' /tmp/yzrs-private/yzrs.json')
if ($LASTEXITCODE -ne 0) { throw 'Device gate or trial start failed; inspect watchdog status' }
