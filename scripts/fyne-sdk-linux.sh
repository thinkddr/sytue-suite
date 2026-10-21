#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Monta en $HOME/fyne-sdk el sysroot gráfico que necesita Fyne para compilar en Linux —
# pkg-config y las cabeceras de X11, Wayland, EGL/GLES, xkbcommon y Vulkan — SIN root:
# descarga los .deb con `apt-get download` y los extrae en un sysroot del propio usuario.
#
# Para qué: el piloto no es una máquina de escritorio y no trae ninguna de esas
# cabeceras, así que el cliente nativo retirado el 2026-09-23 fallaba en los backends
# de xkb y Vulkan. Hoy lo usan los `cmd/` de Fyne, que piden las mismas cabeceras, y
# no hace falta que nadie sea root en el piloto ni en un runner. Se llamaba
# gio-sdk-linux.sh y montaba $HOME/gio-sdk; quien lo tenga lo vuelve a correr (es
# idempotente) y puede borrar el viejo.
#
#   bash scripts/fyne-sdk-linux.sh          # una vez; es idempotente
#   source "$HOME/fyne-sdk/entorno.sh"      # en cada shell que vaya a compilar
#   (cd apps/fyne && CGO_ENABLED=1 go build ./cmd/...)
#
# Solo compila: para EJECUTAR la suite hacen falta además las bibliotecas en tiempo de
# ejecución y un servidor gráfico, que el piloto tampoco tiene.
set -euo pipefail
SDK="$HOME/fyne-sdk"
mkdir -p "$SDK/debs" "$SDK/root" "$SDK/bin"
cd "$SDK/debs"

PAQ="pkgconf pkgconf-bin libpkgconf7
libx11-dev libx11-6 x11proto-dev libxau-dev libxdmcp-dev libxcb1-dev libxcb1
libxcursor-dev libxcursor1 libxfixes-dev libxfixes3 libxrender-dev libxrender1
libxkbcommon-dev libxkbcommon0 libxkbcommon-x11-dev libxkbcommon-x11-0
libxcb-xkb-dev libxcb-xkb1 libxcb-shape0-dev libxcb-shape0 libxcb-xfixes0-dev libxcb-xfixes0
libxcb-render0-dev libxcb-render0 libxcb-randr0-dev libxcb-randr0 libx11-xcb-dev libx11-xcb1
libwayland-dev libwayland-client0 libwayland-cursor0 libwayland-egl1 libwayland-server0
wayland-protocols libffi-dev libffi8
libvulkan-dev libvulkan1
libegl-dev libegl1 libegl-mesa0 libgles-dev libgles2 libglvnd-dev libglvnd-core-dev
libgl-dev libgl1 libglx-dev libglx0 libopengl-dev libopengl0 libglx-mesa0
libxext-dev libxext6
libpipewire-0.3-dev libspa-0.2-dev"

echo "== descargando =="
for p in $PAQ; do
  ls "${p}"_*.deb >/dev/null 2>&1 && continue
  apt-get download "$p" >/dev/null 2>&1 || echo "  (sin $p)"
done

echo "== extrayendo =="
for d in *.deb; do dpkg -x "$d" "$SDK/root" 2>/dev/null || true; done

# pkg-config es pkgconf, y su libpkgconf.so tampoco está en el sistema: se envuelve con
# LD_LIBRARY_PATH en vez de enlazarlo, que es lo que fallaba con un enlace simbólico.
LIBS="$SDK/root/usr/lib/x86_64-linux-gnu"
if [ -x "$SDK/root/usr/bin/pkgconf" ]; then
  printf '#!/bin/sh\nexec env LD_LIBRARY_PATH=%s "%s/pkgconf" "$@"\n' \
    "$LIBS" "$SDK/root/usr/bin" > "$SDK/bin/pkg-config"
  chmod +x "$SDK/bin/pkg-config"
  cp "$SDK/bin/pkg-config" "$SDK/bin/pkgconf"
fi

# Los .pc traen `prefix=/usr`: se reescribe al sysroot o pkg-config devuelve las rutas
# del sistema, que es donde NO están las cabeceras.
find "$SDK/root" -name '*.pc' -exec sed -i "s|^prefix=/usr$|prefix=$SDK/root/usr|" {} +

cat > "$SDK/entorno.sh" <<EOF
export PATH="$SDK/bin:\$HOME/toolchains/go/bin:\$PATH"
export PKG_CONFIG_PATH="$SDK/root/usr/lib/x86_64-linux-gnu/pkgconfig:$SDK/root/usr/share/pkgconfig"
export PKG_CONFIG_SYSROOT_DIR=""
export CGO_CFLAGS="-I$SDK/root/usr/include -I$SDK/root/usr/include/pipewire-0.3 -I$SDK/root/usr/include/spa-0.2"
export CGO_LDFLAGS="-L$SDK/root/usr/lib/x86_64-linux-gnu -Wl,-rpath-link,$SDK/root/usr/lib/x86_64-linux-gnu"
EOF
echo "== listo: source $SDK/entorno.sh =="
"$SDK/bin/pkg-config" --version || true

echo "== módulos que Fyne pide =="
PKG_CONFIG_PATH="$SDK/root/usr/lib/x86_64-linux-gnu/pkgconfig:$SDK/root/usr/share/pkgconfig" \
  sh -c 'for m in egl wayland-egl wayland-client wayland-cursor x11 xkbcommon xkbcommon-x11 x11-xcb xcursor xfixes vulkan; do
    printf "  %-16s " "$m"; '"$SDK/bin/pkg-config"' --exists "$m" && echo OK || echo FALTA
  done'
