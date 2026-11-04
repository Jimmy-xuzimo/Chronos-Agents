"""Create a distributable module inventory and copy original legal notices."""
import json
from pathlib import Path
import shutil
import subprocess
import sys

out = Path(sys.argv[1] if len(sys.argv) > 1 else 'artifacts/third-party')
out.mkdir(parents=True, exist_ok=True)
subprocess.check_call(['go','mod','download'])
raw = subprocess.check_output(['go', 'list', '-m', '-json', 'all'], text=True)
decoder, offset, modules = json.JSONDecoder(), 0, []
while offset < len(raw):
    if not raw[offset:].strip():
        break
    offset += len(raw[offset:]) - len(raw[offset:].lstrip())
    value, used = decoder.raw_decode(raw[offset:]); offset += used
    if value.get('Main'):
        continue
    modules.append({key: value[key] for key in ('Path', 'Version', 'Sum', 'GoVersion') if key in value})
    directory = Path(value['Dir']) if value.get('Dir') else None
    if directory:
        destination = out / (value['Path'].replace('/', '_') + '@' + value.get('Version', ''))
        for source in directory.iterdir():
            if source.is_file() and source.name.upper().startswith(('LICENSE', 'COPYING', 'NOTICE', 'PATENTS')) and source.stat().st_size < 1024 * 1024:
                destination.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(source, destination / source.name)
goroot = Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip())
(out / 'Go').mkdir(exist_ok=True)
for name in ('LICENSE', 'PATENTS'):
    if (goroot / name).is_file():
        shutil.copyfile(goroot / name, out / 'Go' / name)
(out / 'modules.json').write_text(json.dumps(modules, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(f'Dependency inventory: {len(modules)} modules; no local cache paths exported')

from urllib.parse import quote
components = [{'type':'library','name':m['Path'],'version':m['Version'],'purl':'pkg:golang/'+quote(m['Path'],safe='/')+'@'+quote(m['Version'],safe='')} for m in modules if 'Version' in m]
android = Path('artifacts/android-dependencies.json')
if android.exists():
    for m in json.loads(android.read_text('utf-8')):
        components.append({'type':'library','name':m['group']+':'+m['name'],'version':m['version'],'purl':'pkg:maven/'+quote(m['group'],safe='')+'/'+quote(m['name'],safe='')+'@'+quote(m['version'],safe='')})
bom={'bomFormat':'CycloneDX','specVersion':'1.5','version':1,'metadata':{'component':{'type':'application','name':'Chronos Agents','version':'0.1.0'}},'components':components}
(out.parent/'bom.cdx.json').write_text(json.dumps(bom,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
