from pathlib import Path
from html.parser import HTMLParser
import urllib.parse
root=Path(__file__).resolve().parents[1]/'website'
class Links(HTMLParser):
 def handle_starttag(self,tag,attrs):
  for key,value in attrs:
   if key in ('href','src') and value and not urllib.parse.urlsplit(value).scheme and not value.startswith('#'):
    assert (root/urllib.parse.urlsplit(value).path).is_file(),value
Links().feed((root/'index.html').read_text(encoding='utf-8'))
print('Pages static asset references verified')

# All three product pages share the same family navigation and integration guide.
family_html = (Path(__file__).resolve().parents[1]/'website/index.html').read_text(encoding='utf-8')
assert 'aria-label="Chronos 系列项目"' in family_html
for product in ('Monitoring', 'Widgets', 'Agents'):
    assert f'https://jimmy-xuzimo.github.io/Chronos-{product}/' in family_html
    assert f'https://github.com/Jimmy-xuzimo/Chronos-{product}' in family_html
for asset in ('suite.css', 'suite.js'):
    assert (Path(__file__).resolve().parents[1]/'website'/asset).is_file()
assert (Path(__file__).resolve().parents[1]/'docs/CHRONOS_ECOSYSTEM.md').is_file()
print('Chronos family navigation, repository links and integration guide verified.')
