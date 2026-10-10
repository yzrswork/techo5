param(
    [Parameter(Mandatory=$true)][string]$ShowIP,
    [Parameter(Mandatory=$true)][string]$DeviceSerial,
    [Parameter(Mandatory=$true)][string]$SSHKey,
    [Parameter(Mandatory=$true)][string]$KnownHosts,
    [Parameter(Mandatory=$true)][string]$Rootfs,
    [Parameter(Mandatory=$true)][string]$ExpectedSHA256,
    [Parameter(Mandatory=$true)][string]$PrivateDir,
    [switch]$PreflightOnly,
    [switch]$HumanApproved
)
$ErrorActionPreference = 'Stop'
if (!$HumanApproved -and !$PreflightOnly) { throw 'Human Gate approval required immediately before Slot B installation, activation and reboot' }
if ($ShowIP -notmatch '^\d{1,3}(\.\d{1,3}){3}$' -or $DeviceSerial -notmatch '^[A-Z0-9]+$' -or
    $ExpectedSHA256 -notmatch '^[a-f0-9]{64}$') { throw 'Invalid target identity or SHA256' }
$Rootfs = [IO.Path]::GetFullPath($Rootfs)
$PrivateDir = [IO.Path]::GetFullPath($PrivateDir)
$SSHKey = [IO.Path]::GetFullPath($SSHKey)
$KnownHosts = [IO.Path]::GetFullPath($KnownHosts)
if (!(Test-Path -LiteralPath $Rootfs) -or !(Test-Path -LiteralPath $SSHKey) -or !(Test-Path -LiteralPath $KnownHosts)) {
    throw 'Rootfs or verified SSH identity is missing'
}
foreach ($name in @('yzrs.json','pc.json','ptt-key.pem','ptt-cert.pem')) {
    if (!(Test-Path -LiteralPath (Join-Path $PrivateDir $name))) { throw 'Permanent PTT configuration incomplete' }
}
$actual = (Get-FileHash -LiteralPath $Rootfs -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $ExpectedSHA256) { throw 'Rootfs SHA256 mismatch' }
$remote = 'root@'+$ShowIP
$sshOptions = @('-o','KexAlgorithms=curve25519-sha256','-o','BatchMode=yes',
    '-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=8','-i',$SSHKey,
    '-o',('UserKnownHostsFile='+$KnownHosts))
if (!(Get-Command ssh-keygen.exe -ErrorAction SilentlyContinue)) { throw 'OpenSSH ssh-keygen unavailable' }
& ssh-keygen.exe -F $ShowIP -f $KnownHosts | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Device host key is not in the verified known_hosts file' }
$name = [IO.Path]::GetFileName($Rootfs)
$remoteRootfs = '/data/techo5-linux/'+$name
$preflight = 'set -eu; export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; grep -q androidboot.serialno=SERIAL /proc/cmdline; status=$(STORE=/store slotctl status); printf "%s\n" "$status"; printf "%s\n" "$status" | grep -q "^active: a$"; printf "%s\n" "$status" | grep -q "^booted: a$"; printf "%s\n" "$status" | grep -q "^slot a: good"; printf "%s\n" "$status" | grep -q "^slot b: empty"; if awk ''$2 == "/usr/local/bin/techo5" {found=1} END {exit !found}'' /proc/mounts; then exit 1; fi; test ! -e /data/misc/techo5/yzrs.json; test ! -e /data/misc/techo5/yzrs-private; test ! -e REMOTEROOTFS; echo SLOT_B_PRECONDITIONS=PASS'
$preflight = $preflight.Replace('SERIAL',$DeviceSerial).Replace('REMOTEROOTFS',$remoteRootfs)
$preflight | & ssh.exe @sshOptions $remote 'tr -d ''\r'' | sh -s'
if ($LASTEXITCODE -ne 0) { throw 'Device preflight failed; no rootfs/config uploaded' }
if ($PreflightOnly) { Write-Output 'STRICT_SSH_SLOT_B_PREFLIGHT=PASS; DEVICE_UNCHANGED'; return }
& ssh.exe @sshOptions $remote 'umask 077; mkdir -m 700 -p /data/misc/techo5/yzrs-private /data/techo5-linux'
if ($LASTEXITCODE -ne 0) { throw 'Private state directory preparation failed' }
foreach ($pair in @(
    @((Join-Path $PrivateDir 'yzrs.json'),'/data/misc/techo5/yzrs.json'),
    @((Join-Path $PrivateDir 'ptt-cert.pem'),'/data/misc/techo5/yzrs-private/ptt-cert.pem'),
    @((Join-Path $PrivateDir 'ptt-key.pem'),'/data/misc/techo5/yzrs-private/ptt-key.pem'),
    @($Rootfs,$remoteRootfs)
)) {
    & scp.exe -O @sshOptions $pair[0] ($remote+':'+$pair[1])
    if ($LASTEXITCODE -ne 0) { throw 'Strict-host-key SCP failed; Slot B unchanged' }
}
& ssh.exe @sshOptions $remote 'chmod 600 /data/misc/techo5/yzrs.json /data/misc/techo5/yzrs-private/ptt-cert.pem /data/misc/techo5/yzrs-private/ptt-key.pem'
if ($LASTEXITCODE -ne 0) { throw 'Permanent PTT permission setup failed; Slot B unchanged' }
$install = 'set -eu; export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; STORE=/store slotctl install PATHNAME; status=$(STORE=/store slotctl status); printf "%s\n" "$status"; printf "%s\n" "$status" | grep -q "^active: b$"; printf "%s\n" "$status" | grep -q "^booted: a$"; printf "%s\n" "$status" | grep -q "^slot a: good"; printf "%s\n" "$status" | grep -q "^slot b: trial"'
$install = $install.Replace('PATHNAME',$remoteRootfs)
& ssh.exe @sshOptions $remote $install
if ($LASTEXITCODE -ne 0) {
    $restoreA = 'set -eu; export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; status=$(STORE=/store slotctl status); printf "%s\n" "$status"; printf "%s\n" "$status" | grep -q "^booted: a$"; printf "%s\n" "$status" | grep -q "^slot a: good"; STORE=/store slotctl switch a; STORE=/store slotctl status'
    & ssh.exe @sshOptions $remote $restoreA
    if ($LASTEXITCODE -ne 0) { throw 'Slot B install did not verify; stop and use USB recovery' }
    throw 'Slot B install did not verify; active slot returned to A; no reboot requested'
}
$reboot = 'set -eu; export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; sync; reboot'
& ssh.exe @sshOptions $remote $reboot
# SSH commonly closes while the requested reboot begins; the USB console confirms the first boot.
Write-Output ('SLOT_B_TRIAL_AND_REBOOT_REQUESTED rootfs_sha256='+$actual+'; verify first boot by USB console')
