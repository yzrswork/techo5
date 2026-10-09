"""Prepare a local candidate from the accepted image; never access a device."""
import argparse
import hashlib
import io
import pathlib
import re
import tarfile

ACCEPTED = "24129b022a835665b8d5111890ab21f1696778df7c0ea072ca5b089c3584fcee"
DAEMON = "usr/local/bin/techo5"
RELEASE = "etc/techo5-release"


def digest(path):
    with path.open("rb") as stream:
        h = hashlib.sha256()
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
        return h.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", type=pathlib.Path, required=True)
    parser.add_argument("--daemon", type=pathlib.Path, required=True)
    parser.add_argument("--out", type=pathlib.Path, required=True)
    parser.add_argument("--release", required=True)
    args = parser.parse_args()
    if digest(args.baseline) != ACCEPTED:
        raise SystemExit("Accepted baseline SHA256 mismatch")
    if args.out.exists():
        raise SystemExit("Refusing to overwrite an existing candidate")
    binary = args.daemon.read_bytes()
    if binary[:5] != b"\x7fELF\x01" or binary[18:20] != b"\x28\x00":
        raise SystemExit("Daemon must be ARM32 little-endian ELF")
    if not re.fullmatch(r"[A-Za-z0-9 .,():T+_-]+", args.release):
        raise SystemExit("Invalid non-secret release identity")
    replacements = {DAEMON: binary, RELEASE: (args.release + "\n").encode()}
    changed = set()
    args.out.parent.mkdir(parents=True, exist_ok=True)
    with tarfile.open(args.baseline) as source, tarfile.open(args.out, "w:gz") as output:
        for member in source:
            name = member.name.removeprefix("./")
            data = source.extractfile(member) if member.isfile() else None
            if name in replacements:
                if not member.isfile():
                    raise SystemExit("Replacement member is not a regular file")
                raw = replacements[name]
                member.size = len(raw)
                data = io.BytesIO(raw)
                changed.add(name)
            output.addfile(member, data)
    if changed != set(replacements):
        args.out.unlink()
        raise SystemExit("Baseline is missing required members")
    # Compare all contents, link targets and protection metadata before delivery.
    with tarfile.open(args.baseline) as before, tarfile.open(args.out) as after:
        old, new = before.getmembers(), after.getmembers()
        if [m.name for m in old] != [m.name for m in new]:
            raise SystemExit("Rootfs member list changed")
        for a, b in zip(old, new):
            if (a.type, a.mode, a.uid, a.gid, a.linkname, a.mtime) != (b.type, b.mode, b.uid, b.gid, b.linkname, b.mtime):
                raise SystemExit("Rootfs protection metadata changed")
            if a.isfile() and a.name.removeprefix("./") not in replacements:
                if hashlib.sha256(before.extractfile(a).read()).digest() != hashlib.sha256(after.extractfile(b).read()).digest():
                    raise SystemExit("Unrelated rootfs content changed")
    print("ROOTFS_PRESERVATION=PASS; changed only daemon and release identity")
    print("SHA256=" + digest(args.out))


if __name__ == "__main__":
    main()
