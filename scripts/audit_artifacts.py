"""Inspect release payloads separately from source, including nested layers.

Bounded to trusted build output. This is a pattern audit, not a malware scan.
Never extracts files or follows archive paths/symlinks on the host.
"""
import gzip
import io
import re
import sys
import tarfile
import zipfile
from pathlib import Path

patterns = [
    re.compile(rb'gh[pousr]_[A-Za-z0-9]{30,}'),
    re.compile(rb'github_pat_[A-Za-z0-9_]{40,}'),
    re.compile(rb'(?i)[A-Z]:[\\/]Users[\\/][^\\/\s]+'),
]
private_markers = [('xuzi' + 'mo.top').encode(), ('106.15.' + '126.75').encode()]
count = 0
violations = []

def scan(data, label, depth=0):
    global count
    count += 1
    if len(data) > 512 * 1024 * 1024 or depth > 7:
        raise SystemExit('Archive exceeds audit bounds: ' + label)
    if any(p.search(data) for p in patterns) or any(p in data for p in private_markers):
        violations.append(label)
    if data.startswith(b'PK\x03\x04'):
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            for item in archive.infolist():
                if not item.is_dir():
                    if item.file_size > 512 * 1024 * 1024:
                        raise SystemExit('Oversize archive member')
                    scan(archive.read(item), label + '/' + item.filename, depth + 1)
    elif data.startswith(b'\x1f\x8b'):
        with gzip.GzipFile(fileobj=io.BytesIO(data)) as stream:
            inflated = stream.read(512 * 1024 * 1024 + 1)
        scan(inflated, label + '/gzip', depth + 1)
    elif len(data) > 512 and data[257:262] == b'ustar':
        with tarfile.open(fileobj=io.BytesIO(data), mode='r:') as archive:
            for item in archive:
                if item.isfile():
                    if item.size > 512 * 1024 * 1024:
                        raise SystemExit('Oversize tar member')
                    scan(archive.extractfile(item).read(), label + '/' + item.name, depth + 1)

target = Path(sys.argv[1] if len(sys.argv) > 1 else '.runtime/release')
for file in target.iterdir():
    if file.is_file():
        if file.suffix in ('.p12', '.pfx', '.key', '.keystore', '.jks', '.secret'):
            violations.append(file.name)
        scan(file.read_bytes(), file.name)
if violations:
    raise SystemExit('Private-pattern matches; inspect without printing content:\n' + '\n'.join(violations))
print(f'Artifact pattern audit passed: {count} files/layers scanned. Manual review still required.')
