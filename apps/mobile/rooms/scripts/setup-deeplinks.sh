#!/usr/bin/env bash
# Parchea el proyecto Capacitor generado para registrar el esquema propio de Rooms.
# Es idempotente: android/ e ios/ se regeneran y no se versionan.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$HERE/.."
SCHEME="sytue-rooms"

MANIFEST="$ROOT/android/app/src/main/AndroidManifest.xml"
if [ -f "$MANIFEST" ] && ! grep -q "android:scheme=\"$SCHEME\"" "$MANIFEST"; then
  python3 - "$MANIFEST" "$SCHEME" <<'PY'
import re, sys
path, scheme = sys.argv[1], sys.argv[2]
src = open(path, encoding='utf-8').read()
block = f'''
            <intent-filter>
                <action android:name="android.intent.action.VIEW" />
                <category android:name="android.intent.category.DEFAULT" />
                <category android:name="android.intent.category.BROWSABLE" />
                <data android:scheme="{scheme}" />
            </intent-filter>
'''
m = re.search(r'(<activity\b[^>]*android:name="\.MainActivity".*?)(\n\s*</activity>)', src, re.S)
if not m:
    sys.exit('Android: no se encontro MainActivity')
open(path, 'w', encoding='utf-8').write(src[:m.end(1)] + block + src[m.end(1):])
PY
fi

PLIST="$ROOT/ios/App/App/Info.plist"
if [ -f "$PLIST" ] && ! /usr/libexec/PlistBuddy -c "Print :CFBundleURLTypes" "$PLIST" >/dev/null 2>&1; then
  /usr/libexec/PlistBuddy \
    -c "Add :CFBundleURLTypes array" \
    -c "Add :CFBundleURLTypes:0 dict" \
    -c "Add :CFBundleURLTypes:0:CFBundleURLName string eu.sytue.rooms" \
    -c "Add :CFBundleURLTypes:0:CFBundleURLSchemes array" \
    -c "Add :CFBundleURLTypes:0:CFBundleURLSchemes:0 string $SCHEME" \
    "$PLIST"
fi

echo "Deep link $SCHEME:// registrado donde existe proyecto nativo."
