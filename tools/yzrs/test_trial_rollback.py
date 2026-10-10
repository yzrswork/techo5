"""Exercise the actual rollback body in an isolated filesystem with fake mount tools."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


class RollbackTests(unittest.TestCase):
    def test_busy_executable_detaches_before_supervisor_restart(self):
        shell = shutil.which('sh')
        if not shell and os.name == 'nt':
            shell = r'C:\Program Files\Git\bin\bash.exe'
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            shell_root = root.as_posix()
            if os.name == 'nt':
                shell_root = '/' + shell_root[0].lower() + shell_root[2:]
            state = root / 'data/misc/techo5'
            backup = state / 'yzrs-trial-backup'
            backup.mkdir(parents=True)
            (backup / 'config.prev').write_text('original')
            (state / 'yzrs.json').write_text('trial')
            (root / 'overlay').touch()
            mock = root / 'bin'
            mock.mkdir()
            commands = {
                'awk': 'test -e "$TRIAL_TEST_ROOT/overlay"',
                'umount': 'echo "umount $*" >> "$TRIAL_TEST_ROOT/order"; test "$1" = -l || exit 1; rm "$TRIAL_TEST_ROOT/overlay"',
                'killall': 'test ! -e "$TRIAL_TEST_ROOT/overlay"; echo killall >> "$TRIAL_TEST_ROOT/order"',
                'iptables-legacy': 'exit 0',
            }
            for name, body in commands.items():
                file = mock / name
                file.write_text('#!/bin/sh\nset -eu\n' + body + '\n')
                file.chmod(0o700)
            source = Path(__file__).with_name('trial-bind.sh').read_text()
            body = source.split("<<'EOF'\n", 1)[1].split('\nEOF', 1)[0]
            body = body.replace('export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin', 'export PATH="$TRIAL_TEST_ROOT/bin:$PATH"')
            body = body.replace('/data/misc/techo5', shell_root + '/data/misc/techo5')
            script = root / 'rollback.sh'
            script.write_text(body)
            env = dict(os.environ, TRIAL_TEST_ROOT=shell_root)
            subprocess.run([shell, script.as_posix()], env=env, check=True, capture_output=True)
            self.assertEqual((state / 'yzrs.json').read_text(), 'original')
            self.assertFalse((root / 'overlay').exists())
            self.assertEqual((root / 'order').read_text().splitlines(), [
                'umount /usr/local/bin/techo5', 'umount -l /usr/local/bin/techo5', 'killall'])


if __name__ == '__main__':
    unittest.main()
