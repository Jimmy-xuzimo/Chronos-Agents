"""Verify local shared tokens; compare companion checkout when available."""
from pathlib import Path
import hashlib
root=Path(__file__).resolve().parents[1]
raw=(root/'design/desktop-design.json').read_text(encoding='utf-8').replace('\r\n','\n')
expected=(root/'design/desktop-design.sha256').read_text().strip()
assert hashlib.sha256(raw.encode()).hexdigest()==expected,'Design manifest mismatch'
other=root.parent/('chronos-agents' if root.name=='chronos-widgets' else 'chronos-widgets')/'design/desktop-design.json'
if other.is_file():assert other.read_text(encoding='utf-8').replace('\r\n','\n')==raw,'Companion design tokens drifted'
print('Shared desktop design tokens verified.')
