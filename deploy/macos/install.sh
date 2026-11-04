#!/bin/sh
set -eu
source_app="${1:?Pass path to the built Chronos Agent.app}"
test -d "$source_app/Contents/MacOS" || { echo 'Invalid application package' >&2; exit 2; }
destination="$HOME/Applications/Chronos Agent.app"
test ! -e "$destination" || { echo 'Existing installation found. Stop its LaunchAgent and move it aside before upgrading.' >&2; exit 2; }
mkdir -p "$HOME/Applications"
ditto "$source_app" "$destination"
echo "Installed to $destination"
echo 'Run the app directly; optional login startup: install.sh APP_PATH --autostart'
if test "${2:-}" = --autostart; then
  mkdir -p "$HOME/Library/LaunchAgents"
  # plistlib escapes user paths correctly, including ampersands and quotes.
  python3 - "$destination" "$HOME/Library/LaunchAgents/dev.chronos.agents.plist" <<'PY'
import plistlib, sys
with open(sys.argv[2], 'wb') as f:
    plistlib.dump({'Label':'dev.chronos.agents','ProgramArguments':[sys.argv[1]+'/Contents/MacOS/chronos-agent'],'RunAtLoad':True,'KeepAlive':False},f)
PY
  launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/dev.chronos.agents.plist"
fi
