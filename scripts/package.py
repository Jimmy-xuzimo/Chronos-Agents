"""Stage explicit release files only; never archive an entire working tree."""
import argparse
import hashlib
import shutil
import tarfile
import zipfile
from pathlib import Path

root = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--output', default='.runtime/release')
args = parser.parse_args()
out = root / args.output
out.mkdir(parents=True, exist_ok=True)
version = '0.1.0'
artifacts = root / 'artifacts'
legal = [(root / name, name) for name in ('LICENSE', 'NOTICE', 'THIRD_PARTY_NOTICES.md', 'README.md')]
for directory in ('docs', 'protocol'):
    legal += [(file, file.relative_to(root).as_posix()) for file in (root / directory).rglob('*') if file.is_file() and file.suffix in ('.md', '.png', '.json')]
legal += [(file, 'third-party/' + file.relative_to(artifacts / 'third-party').as_posix()) for file in (artifacts / 'third-party').rglob('*') if file.is_file()]
legal += [(artifacts / 'bom.cdx.json', 'bom.cdx.json')]

for platform, arch in [('windows', 'amd64'), ('linux', 'amd64'), ('linux', 'arm64')]:
    suffix = '.exe' if platform == 'windows' else ''
    members = [(artifacts / f'chronos-{component}-{platform}-{arch}{suffix}', f'chronos-{component}{suffix}') for component in ('agent', 'hub')]
    members += legal
    if platform == 'linux':
        members += [(file, file.relative_to(root).as_posix()) for folder in ('deploy/systemd', 'deploy/docker') for file in (root / folder).glob('*') if file.suffix in ('.service', '.yml')]
    for source, _ in members:
        if not source.is_file():
            raise SystemExit('Missing staged input: ' + source.name)
    name = f'chronos-agents-{version}-{platform}-{arch}'
    if platform == 'windows':
        with zipfile.ZipFile(out / (name + '.zip'), 'w', zipfile.ZIP_DEFLATED) as archive:
            for source, destination in members:
                archive.write(source, destination)
    else:
        with tarfile.open(out / (name + '.tar.gz'), 'w:gz') as archive:
            for source, destination in members:
                info = archive.gettarinfo(str(source), destination)
                info.uid = info.gid = 0; info.uname = info.gname = ''
                info.mode = 0o755 if destination in ('chronos-agent', 'chronos-hub') else 0o644
                with source.open('rb') as stream:
                    archive.addfile(info, stream)
setup = artifacts / f'Chronos-Agent-{version}-windows-x64-setup.exe'
shutil.copyfile(setup, out / setup.name)
apk = root / 'clients/android/app/build/outputs/apk/release/app-release.apk'
if apk.exists():
    shutil.copyfile(apk, out / f'chronos-agent-{version}-android.apk')
shutil.copyfile(artifacts / 'bom.cdx.json', out / 'bom.cdx.json')
print('Staged explicit application artifacts. Add verified macOS/images before final checksums.')

def checksums():
    lines = []
    for file in sorted(out.iterdir()):
        if file.is_file() and file.name != 'SHA256SUMS.txt':
            lines.append(hashlib.file_digest(file.open('rb'), 'sha256').hexdigest() + '  ' + file.name)
    (out / 'SHA256SUMS.txt').write_text('\n'.join(lines) + '\n', encoding='utf-8')

checksums()
