#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Regenera el corpus de fidelidad «de LibreOffice» a partir de las fuentes planas
# (fuentes/*.fodt|fods|fodp), que son las que se editan. Los ficheros de «sabor
# Microsoft» (office/*) son XML escrito a mano y el test los empaqueta al vuelo: no
# pasan por aquí.
#
# Se ejecuta en el contenedor de los agentes del frontal, NUNCA en el portátil:
#   ssh sytue-rooms-media "agente <worktree> 'services/docs/internal/domain/testdata/fidelidad/generar.sh'"
#
# SOFFICE elige el binario. El contenedor solo trae Calc; para Writer e Impress hay que
# desempaquetar los .deb de la misma versión en un prefijo del disco de los agentes
# (apt-get download libreoffice-{writer,impress,draw,core,calc}-nogui libreoffice-common
# libreoffice-base-core ure y las bibliotecas que pida `apt-get install -s`; dpkg -x; copiar
# encima /usr/lib/libreoffice y /etc/libreoffice sin pisar; enlazar los .xcd de
# share/.registry en etc/libreoffice/registry; y apuntar program/fundamentalrc al prefijo)
# y lanzar su soffice con LD_LIBRARY_PATH=<prefijo>/usr/lib/aarch64-linux-gnu.
#
# Después: regenerar la matriz (ver docs/architecture.md#fidelidad-ofimatica).
set -eu
cd "$(dirname "$0")"
SOFFICE="${SOFFICE:-soffice}"
perfil="$(mktemp -d)"
trap 'rm -rf "$perfil"' EXIT
conv() { "$SOFFICE" -env:UserInstallation="file://$perfil" --headless --convert-to "$1" --outdir libreoffice "$2" >/dev/null; }
mkdir -p libreoffice
conv 'docx:MS Word 2007 XML' fuentes/documento.fodt
conv odt fuentes/documento.fodt
conv 'xlsx:Calc MS Excel 2007 XML' fuentes/hoja.fods
conv ods fuentes/hoja.fods
conv 'pptx:Impress MS PowerPoint 2007 XML' fuentes/presentacion.fodp
conv odp fuentes/presentacion.fodp
conv 'pptx:Impress MS PowerPoint 2007 XML' fuentes/diapos-avanzada.fodp
conv odp fuentes/diapos-avanzada.fodp
ls -l libreoffice
