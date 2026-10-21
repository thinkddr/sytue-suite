#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Trae el corpus de TestCorpusTerceros (corpus_terceros_test.go): los ficheros de prueba .xlsx y
# .pptx que publican proyectos con licencia permisiva (Apache-2.0, MIT, BSD). Nunca GPL ni AGPL:
# la primera línea de cada licencia queda en $C/LICENCIAS.tsv para comprobarlo, y un proyecto que
# cambie a una licencia así sale de la lista. Deja cada fichero en
# $C/{xlsx,pptx}/<proyecto>__<ruta-aplanada>. Pesa unos 230 MB: no va al repositorio.
#
#   ./traer.sh /ruta/del/corpus
set -u
C=${1:?uso: traer.sh /ruta/del/corpus}
mkdir -p "$C/src" "$C/xlsx" "$C/pptx"
: > "$C/LICENCIAS.tsv"
traer() { # nombre url ruta...
  local n=$1 url=$2; shift 2
  local d="$C/src/$n"
  if [ ! -d "$d/.git" ]; then
    git clone -q --depth 1 --filter=blob:none --sparse "$url" "$d" || { echo "FALLO $n"; return; }
    git -C "$d" sparse-checkout set --no-cone "$@" '/LICENSE*' '/COPYING*' '/License*' '/LICENCE*' >/dev/null 2>&1
  fi
  local lic=$(ls "$d" | grep -i -E '^(licen[cs]e|copying)' | head -1)
  printf '%s\t%s\t%s\n' "$n" "$url" "$(head -c 300 "$d/$lic" 2>/dev/null | tr '\n\t' '  ' | sed 's/  */ /g' | cut -c1-120)" >> "$C/LICENCIAS.tsv"
  for ext in xlsx xlsm pptx pptm; do
    local dst=$C/${ext:0:3}x; [ "$ext" = xlsm ] && dst=$C/xlsx; [ "$ext" = pptm ] && dst=$C/pptx
    find "$d" -path "$d/.git" -prune -o -type f -iname "*.$ext" -print | while read -r f; do
      rel=${f#"$d/"}; cp "$f" "$dst/${n}__$(echo "$rel" | tr '/ ' '__')"
    done
  done
}
traer poi https://github.com/apache/poi.git '/test-data/spreadsheet/' '/test-data/slideshow/' '/test-data/xmldsign/' '/test-data/integration/'
traer xlsxwriter https://github.com/jmcnamara/XlsxWriter.git '/xlsxwriter/test/comparison/xlsx_files/'
traer python-pptx https://github.com/scanny/python-pptx.git '/tests/test_files/' '/features/steps/test_files/'
traer openxml-sdk https://github.com/dotnet/Open-XML-SDK.git '/test/'
traer closedxml https://github.com/ClosedXML/ClosedXML.git '/ClosedXML.Tests/Resource/'
traer calamine https://github.com/tafia/calamine.git '/tests/'
traer excelize https://github.com/qax-os/excelize.git '/test/'
traer exceljs https://github.com/exceljs/exceljs.git '/spec/'
traer tealeg-xlsx https://github.com/tealeg/xlsx.git '/testdocs/'
traer shapecrawler https://github.com/ShapeCrawler/ShapeCrawler.git '/tests/' '/test/'
traer phpspreadsheet https://github.com/PHPOffice/PhpSpreadsheet.git '/tests/data/'
traer exceldatareader https://github.com/ExcelDataReader/ExcelDataReader.git '/src/TestData/' '/test/'
traer xlnt https://github.com/tfussell/xlnt.git '/tests/data/'
traer tika https://github.com/apache/tika.git '*.xlsx' '*.xlsm' '*.pptx' '*.pptm'
traer openxml-powertools https://github.com/OfficeDev/Open-XML-PowerTools.git '/TestFiles/'
traer rust-xlsxwriter https://github.com/jmcnamara/rust_xlsxwriter.git '/tests/input/'
echo "xlsx: $(ls "$C/xlsx" | wc -l)  pptx: $(ls "$C/pptx" | wc -l)  $(du -sh "$C/xlsx" "$C/pptx" | tr '\n' ' ')"
