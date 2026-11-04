"""Check intended public sources; binary audit remains a separate step."""
import re
import subprocess
from pathlib import Path

root = Path(__file__).resolve().parents[1]
files = subprocess.check_output(['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z'], cwd=root).decode().split('\0')
violations = []
patterns = [
    re.compile(rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----'),
    re.compile(rb'gh[pousr]_[A-Za-z0-9]{30,}'),
    re.compile(rb'github_pat_[A-Za-z0-9_]{40,}'),
    re.compile(rb'(?:[A-Z]:[\\/])Users[\\/][^\\/\s]+', re.I),
]
# User-owned deployment suffix assembled so this audit does not flag itself.
private_markers = [('xuzi'+'mo.top').encode(), ('106.15.'+'126.75').encode()]
for name in files:
    if not name:
        continue
    path = root / name
    if path.suffix.lower() in ('.exe', '.apk', '.aab', '.pfx', '.jks', '.keystore', '.secret', '.sqlite', '.db') or any(part in ('.runtime', '.data', 'node_modules') for part in path.parts):
        violations.append(name + ': private/generated file'); continue
    if path.stat().st_size > 4 * 1024 * 1024:
        violations.append(name + ': unexpectedly large source'); continue
    data = path.read_bytes()
    if any(p.search(data) for p in patterns) or any(p in data for p in private_markers):
        violations.append(name + ': private value or credential pattern')
if violations:
    raise SystemExit('\n'.join(violations))
print(f'Public source checks passed: {sum(bool(n) for n in files)} files. Manual review still required.')
