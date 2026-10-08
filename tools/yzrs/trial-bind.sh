#!/bin/sh
# Human-approved temporary daemon test. Never changes rootfs slots or boot/recovery.
set -eu
binary=${1:?uploaded binary path}
expected=${2:?locally verified SHA256}
config=${3:?uploaded private config path}
ttl=${4:-300}
case "$binary" in /tmp/yzrs-*.elf) ;; *) echo 'STOP: temporary binary path required'; exit 1;; esac
case "$config" in /tmp/yzrs-private/yzrs.json) ;; *) echo 'STOP: private config path required'; exit 1;; esac
case "$ttl" in 300) ;; *) echo 'STOP: fixed five minute rollback'; exit 1;; esac
case "$expected" in *[!a-f0-9]*|'') echo 'STOP: invalid SHA256'; exit 1;; esac
test "${#expected}" -eq 64
test -x "$binary"
test -s "$config"
test -s /tmp/yzrs-private/ptt-key.pem
test -s /tmp/yzrs-private/ptt-cert.pem
status=$(STORE=/store slotctl status)
printf '%s\n' "$status" | grep -q '^active: a$'
printf '%s\n' "$status" | grep -q '^booted: a$'
printf '%s\n' "$status" | grep -q '^slot a: good'
printf '%s\n' "$status" | grep -q '^slot b: empty'
actual=$(sha256sum "$binary" | cut -d ' ' -f 1)
test "$actual" = "$expected"
# Refuse to stack trials or alter an existing daemon overlay.
test ! -e /tmp/yzrs-trial
test ! -e /data/misc/techo5/yzrs-trial-backup
if awk '$2 == "/usr/local/bin/techo5" {found=1} END {exit !found}' /proc/mounts; then
    echo 'STOP: daemon already has a bind mount'; exit 1
fi
mkdir -m 700 /tmp/yzrs-trial
mkdir -m 700 /data/misc/techo5/yzrs-trial-backup
state=/data/misc/techo5/yzrs.json
if test -e "$state"; then
    cp -p "$state" /data/misc/techo5/yzrs-trial-backup/config.prev
else
    touch /data/misc/techo5/yzrs-trial-backup/config.absent
fi
cat > /tmp/yzrs-trial/rollback.sh <<'EOF'
#!/bin/sh
set -eu
state=/data/misc/techo5/yzrs.json
if awk '$2 == "/usr/local/bin/techo5" {found=1} END {exit !found}' /proc/mounts; then
    umount /usr/local/bin/techo5
fi
if test -e /data/misc/techo5/yzrs-trial-backup/config.prev; then
    cp -p /data/misc/techo5/yzrs-trial-backup/config.prev "$state"
elif test -e /data/misc/techo5/yzrs-trial-backup/config.absent; then
    rm -f "$state"
fi
killall techo5 || true
# A process crash may bypass its firewall cleanup; remove only the YZRS-owned chain.
iptables-legacy -D TECHO5-IN -j YZRS-PTT 2>/dev/null || true
iptables-legacy -F YZRS-PTT 2>/dev/null || true
iptables-legacy -X YZRS-PTT 2>/dev/null || true
echo 'Original daemon restored; slots untouched.'
EOF
chmod 700 /tmp/yzrs-trial/rollback.sh
# Keep recovery available even if the device reboots before the watchdog runs.
cp /tmp/yzrs-trial/rollback.sh /data/misc/techo5/yzrs-trial-backup/rollback.sh
# Independent watchdog survives SSH disconnection and a daemon failure.
nohup sh -c 'sleep 300; /tmp/yzrs-trial/rollback.sh' </dev/null >/tmp/yzrs-trial/watchdog.log 2>&1 &
cp "$config" "$state"
chmod 600 "$state"
mount --bind "$binary" /usr/local/bin/techo5
killall techo5 || true
echo 'Five-minute daemon trial started. Automatic rollback is armed; Slot A is unchanged.'
