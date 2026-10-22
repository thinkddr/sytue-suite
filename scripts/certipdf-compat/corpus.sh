#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Prepara el CORPUS y las HERRAMIENTAS de la medida de compatibilidad de CertiPDF
# (medir.py + pdfcompat_medida_test.go). Va en el frontal, dentro del contenedor
# de los agentes, nunca en el portátil ni en el sistema del frontal:
#
#   agente /srv/agentes/wt/<árbol> 'scripts/certipdf-compat/corpus.sh /srv/agentes/tmp/certipdf-corpus'
#
# Qué baja, y con qué licencia. Solo FICHEROS DE PRUEBA, nunca código: los
# clones son dispersos y piden únicamente *.pdf y la licencia
# (docs/security.md §4, clean-room).
#   * pdf.js test/pdfs (Apache-2.0): solo los que vienen en el repositorio, no
#     los enlazados. Y test_manifest.json, que es donde publica las contraseñas.
#   * Apache PDFBox (Apache-2.0) y qpdf qtest (Apache-2.0).
#   * veraPDF-corpus (CC BY 4.0): se cita en el informe.
#   * PDF públicos de administraciones de la UE descargables sin sesión (ue.py).
# Herramientas (caja negra, en <dir>/herramientas): qpdf de Debian
# desempaquetado sin instalar, un venv con pyHanko, pypdf, numpy y Pillow, y
# veraPDF. Poppler (pdftoppm…) ya viene en el contenedor.
set -euo pipefail
dir="${1:?uso: corpus.sh <directorio>}"
aqui="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$dir/fuentes" "$dir/herramientas"
cd "$dir/fuentes"

clonar() { # destino url patrones…
  local d=$1 u=$2
  shift 2
  [ -d "$d" ] && return 0
  git clone -q --depth 1 --filter=blob:none --no-checkout "$u" "$d"
  git -C "$d" sparse-checkout set --no-cone "$@"
  git -C "$d" checkout -q
  echo "$d: $(find "$d" -name '*.pdf' | wc -l) pdf"
}
clonar pdfjs https://github.com/mozilla/pdf.js '/test/pdfs/*.pdf' /test/test_manifest.json /LICENSE
clonar pdfbox https://github.com/apache/pdfbox '*.pdf' /LICENSE.txt /NOTICE.txt
clonar qpdf https://github.com/qpdf/qpdf '/qpdf/qtest/qpdf/*.pdf' /LICENSE.txt /NOTICE.md
[ -d verapdf ] || git clone -q --depth 1 https://github.com/veraPDF/veraPDF-corpus verapdf
python3 -I "$aqui/ue.py" ue

cd "$dir/herramientas"
if [ ! -x qpdf/usr/bin/qpdf ]; then
  mkdir -p deb && (cd deb && apt-get download qpdf libqpdf29)
  for f in deb/*.deb; do dpkg -x "$f" qpdf; done
fi
if [ ! -x py/bin/python ]; then
  python3 -m venv py
  py/bin/pip -q install --upgrade pip
  py/bin/pip -q install pyhanko pyhanko-cli pypdf numpy pillow
fi
[ -x verapdf/inst/verapdf ] || bash "$aqui/../verapdf-instalar.sh" "$dir/herramientas/verapdf"
# Las contraseñas de los PDF cifrados de pdf.js, por nombre de fichero.
py/bin/python -I -c '
import json, sys
m = json.load(open(sys.argv[1]))
json.dump({x["file"].rsplit("/", 1)[-1]: x["password"] for x in m if x.get("password")}, open("claves.json", "w"), ensure_ascii=False)
' "$dir/fuentes/pdfjs/test/test_manifest.json"
echo "corpus en $dir/fuentes; herramientas en $dir/herramientas"
