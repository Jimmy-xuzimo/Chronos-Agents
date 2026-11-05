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
