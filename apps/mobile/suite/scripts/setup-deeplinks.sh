#!/usr/bin/env bash
# Registra el esquema de deep link del PRODUCTO ACTIVO en los proyectos nativos iOS y
# Android. El producto sale de SYTUE_APP (tabla en products.ts); sin variable, la suite.
#
#   SYTUE_APP=mail bash scripts/setup-deeplinks.sh    # registra sytue-mail://
#
# Cada app registra SOLO el suyo: si todas declararan `sytue://`, el sistema operativo
# elegiria una arbitrariamente y «abrir en la aplicacion» abriria la que no toca.
#
# Por que un script y no un fichero versionado: `ios/` y `android/` los GENERA
# `npx cap add` y estan en .gitignore, asi que su AndroidManifest.xml e Info.plist no se
# pueden versionar — se regeneran. Esto los parchea despues, y es IDEMPOTENTE.
#
# Uso:  npx cap add ios && npx cap add android && bash scripts/setup-deeplinks.sh
#
# El destino de cada enlace lo valida el lado web (sameSuite en @sytue/shell) antes de
# navegar: un deep link lo puede emitir cualquiera y el sistema no comprueba quien.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$HERE/.."

# Esquema e identificador siguen una REGLA, no una tabla: `one` es la suite y se queda con
# `sytue://` a secas; el resto son `sytue-<clave>://` y `eu.sytue.<clave>`. Aplicarla aqui
# es una linea; copiar la tabla de products.ts serian diez, y se desincronizarian.
#
# La clave se valida contra products.ts, que es donde vive la lista: asi un SYTUE_APP mal
# escrito falla aqui en vez de registrar un esquema que no existe en ninguna parte.
APP="${SYTUE_APP:-one}"
if ! grep -Eq "key: [\"']${APP}[\"']" "$ROOT/products.ts"; then
  echo "SYTUE_APP=\"$APP\" no esta en products.ts" >&2
  grep -oE "key: [\"'][a-z]+[\"']" "$ROOT/products.ts" | sed -E "s/key: /  - /;s/[\"']//g" >&2
  exit 1
fi
[ "$APP" = "one" ] && SCHEME="sytue" || SCHEME="sytue-$APP"
BUNDLE="eu.sytue.$APP"
echo "Producto: $APP — esquema $SCHEME, paquete $BUNDLE"

# --- Android -----------------------------------------------------------------------
# Un <intent-filter> con BROWSABLE: sin esa categoria el sistema no entrega enlaces
# pulsados en el navegador, que es justo el caso de uso.
MANIFEST="$ROOT/android/app/src/main/AndroidManifest.xml"
if [ -f "$MANIFEST" ]; then
  if grep -q "android:scheme=\"$SCHEME\"" "$MANIFEST"; then
    echo "Android: ya registrado"
  else
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
# Se cuelga de la MainActivity, que es la que Capacitor entrega al plugin App.
m = re.search(r'(<activity\b[^>]*android:name="\.MainActivity".*?)(\n\s*</activity>)', src, re.S)
if not m:
    sys.exit('Android: no se encontro MainActivity en el manifest')
open(path, 'w', encoding='utf-8').write(src[:m.end(1)] + block + src[m.end(1):])
print('Android: intent-filter anadido')
PY
  fi
else
  echo "Android: sin proyecto nativo (ejecuta 'npx cap add android')"
fi

# --- iOS ---------------------------------------------------------------------------
PLIST="$ROOT/ios/App/App/Info.plist"
if [ -f "$PLIST" ]; then
  if /usr/libexec/PlistBuddy -c "Print :CFBundleURLTypes" "$PLIST" >/dev/null 2>&1; then
    echo "iOS: ya registrado"
  else
    /usr/libexec/PlistBuddy \
      -c "Add :CFBundleURLTypes array" \
      -c "Add :CFBundleURLTypes:0 dict" \
      -c "Add :CFBundleURLTypes:0:CFBundleURLName string $BUNDLE" \
      -c "Add :CFBundleURLTypes:0:CFBundleURLSchemes array" \
      -c "Add :CFBundleURLTypes:0:CFBundleURLSchemes:0 string $SCHEME" \
      "$PLIST"
    echo "iOS: CFBundleURLTypes anadido"
  fi
else
  echo "iOS: sin proyecto nativo (requiere macOS + Xcode; 'npx cap add ios')"
fi

echo "Listo. Recuerda 'npx cap sync' para propagar la config."
