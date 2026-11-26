#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Construye SytueIA.framework, el puente de Sytue Office con Foundation Models
# (SytueIA.swift), para el iPhone o el Simulador, SIN proyecto de Xcode: swiftc y un
# Info.plist. Solo en macOS con un Xcode cuyo SDK traiga FoundationModels (Xcode 26+).
#
#   construir.sh <iphoneos|iphonesimulator> <carpeta de salida> [mínimo de iOS]
#
# Deja <salida>/SytueIA.framework SIN FIRMAR: lo firma empaquetar.sh con la identidad de
# la app, dentro de <App>.app/Frameworks/, antes de firmar el .app.
#
# - El mínimo es el de la app (Sytue.xcconfig) y no 26: el framework carga en cualquier
#   iOS que la app admita, y Foundation Models va enlazado DÉBIL (`-weak_framework`) y
#   tras `#available(iOS 26)`. En un iOS anterior, `sytue_ia_estado` dice 5 y ya.
# - `-install_name @rpath/…`: Go no lo enlaza, lo abre con dlopen por su ruta dentro del
#   bundle (sistema_ios.go), pero el nombre tiene que ser el de un framework embebido o
#   App Store Connect lo rechaza.
set -euo pipefail

sdk="${1:?uso: construir.sh <iphoneos|iphonesimulator> <salida> [mínimo]}"
salida="${2:?falta la carpeta de salida}"
aqui="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
minimo="${3:-$(sed -n 's/^IPHONEOS_DEPLOYMENT_TARGET = \([0-9][0-9.]*\)$/\1/p' "$aqui/../Sytue.xcconfig")}"
[ -n "$minimo" ] || { echo "::error::sin mínimo de iOS (Sytue.xcconfig)" >&2; exit 1; }

case "$sdk" in
iphoneos) objetivo="arm64-apple-ios$minimo" plataforma=iPhoneOS ;;
iphonesimulator) objetivo="arm64-apple-ios$minimo-simulator" plataforma=iPhoneSimulator ;;
*) echo "::error::SDK «$sdk»: iphoneos o iphonesimulator" >&2; exit 1 ;;
esac

{ [ "$(uname -s)" = Darwin ] && command -v xcrun >/dev/null 2>&1; } || {
  echo "::error::SytueIA.framework solo se construye en macOS con Xcode (runner sytue-macos o Xcode Cloud)." >&2
  exit 1
}
raiz_sdk="$(xcrun --sdk "$sdk" --show-sdk-path)"
[ -d "$raiz_sdk/System/Library/Frameworks/FoundationModels.framework" ] || {
  echo "::error::el SDK $sdk de este Xcode no trae FoundationModels: hace falta Xcode 26 o posterior." >&2
  exit 1
}

fw="$salida/SytueIA.framework"
rm -rf "$fw"
mkdir -p "$fw"
xcrun --sdk "$sdk" swiftc -parse-as-library -emit-library -module-name SytueIA \
  -target "$objetivo" -sdk "$raiz_sdk" -swift-version 5 -O \
  -Xlinker -install_name -Xlinker @rpath/SytueIA.framework/SytueIA \
  -Xlinker -weak_framework -Xlinker FoundationModels \
  -o "$fw/SytueIA" "$aqui/SytueIA.swift"

cat >"$fw/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
	<key>CFBundleDevelopmentRegion</key><string>es</string>
	<key>CFBundleExecutable</key><string>SytueIA</string>
	<key>CFBundleIdentifier</key><string>eu.sytue.office.ia</string>
	<key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
	<key>CFBundleName</key><string>SytueIA</string>
	<key>CFBundlePackageType</key><string>FMWK</string>
	<key>CFBundleShortVersionString</key><string>1.0</string>
	<key>CFBundleVersion</key><string>1</string>
	<key>CFBundleSupportedPlatforms</key><array><string>$plataforma</string></array>
	<key>MinimumOSVersion</key><string>$minimo</string>
</dict></plist>
PLIST

# Lo que de verdad exporta: las tres funciones que busca Go. La salida de nm y otool, en
# una variable y no en tubería hacia `grep -q`: grep sale al primer acierto, el que
# escribe recibe SIGPIPE y pipefail lo daría por fallo (.github/tests/ci-sin-azar-test.sh).
simbolos="$(xcrun nm -gU "$fw/SytueIA")"
for s in sytue_ia_estado sytue_ia_responder sytue_ia_cancelar; do
  grep -q " _$s\$" <<<"$simbolos" || { echo "::error::SytueIA no exporta $s" >&2; exit 1; }
done
# Y FoundationModels débil: enlazado fuerte, la app no abriría en iOS 15–25.
debiles="$(xcrun otool -l "$fw/SytueIA" | awk '/LC_LOAD_WEAK_DYLIB/{d=1} d && /name/{print; d=0}')"
grep -q FoundationModels <<<"$debiles" || {
  echo "::error::SytueIA enlaza FoundationModels fuerte: no abriría antes de iOS 26" >&2
  exit 1
}
echo "$fw"
