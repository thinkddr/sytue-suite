#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# ¿LE QUEDA MARGEN AL RELOJ DE CADA JOB?
#
# `timeout-minutes` es el único número del CI que no avisa cuando se queda
# corto: el job va tardando más cada semana, sigue en verde, y un día sale
# «The runner cancelled the job because it exceeds the maximum run time» con el
# trabajo al 95 %. Ha pasado dos veces:
#
#   · ci-go, v2.77.3 → v2.83.1: nueve releases seguidas cortadas a los 25:00
#     exactos, y subirlo a 45 solo aplazó lo mismo.
#   · ci-gio, v3.4.0 (2026-09-21): 20:00 exactos, con el gate anti-slop ya en
#     verde. Lo que se lee arriba es un «context deadline exceeded» copiando
#     SUMMARY.md, que es el efecto y no se parece nada a su causa.
#
# Esto lo detecta ANTES, comparando lo declarado con lo MEDIDO: para cada job
# que de verdad corrió y salió en verde, el reloj tiene que ser al menos el
# doble de lo que tardó. Es el criterio de la PR #727 y el de ci-gio.
#
# La medida sale de `started_at`/`completed_at` del JOB —en GitHub desde el
# 2026-09-27; en la forja eran `created_at`/`updated_at` de la tarea—, que es
# cuándo el runner lo cogió y cuándo lo soltó, no cuándo se encoló el run.
# Comprobado en la forja contra la primera y la última marca del log, que es
# el reloj exacto:
#
#   ci-go 2528 vet-build   213 s vs 210 s      ci-gio 2527 gio  1206 s vs 1203 s
#   ci-go 2528 test-race   483 s vs 477 s      ci-gio 2507 gio  1140 s vs 1118 s
#
# Entre 2 y 22 segundos de más sobre ejecuciones de 3 a 20 minutos, y siempre
# de más: el error va del lado seguro y cuesta una petición en vez de bajarse
# los megas de log de cada job.
#
# Uso:
#   GITHUB_TOKEN=… scripts/ci-margen-de-reloj.sh          # últimos 40 runs en verde
#   GITHUB_TOKEN=… scripts/ci-margen-de-reloj.sh 400
#   SYTUE_CI_MARGEN=3 scripts/ci-margen-de-reloj.sh      # exigir el triple
#
# 40 runs son las últimas horas, que es lo que contesta «¿está sano AHORA?».
# Una ventana ancha mira más atrás y saca cosas que ya no son ciertas —con 400,
# los jobs de Go de antes de montar la caché compartida, que hoy tardan la
# mitad—; sirve para ver de dónde viene un job, no para decidir su reloj.
#
# Solo lee: no despacha nada, no cancela nada y no escribe en la forja.
set -uo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API="${SYTUE_GIT_API:-https://api.github.com}"
REPO="${SYTUE_GIT_REPO:-thinkddr/sytue-one}"
TAREAS="${1:-40}"
MARGEN="${SYTUE_CI_MARGEN:-2}"

TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-${SYTUE_GIT_TOKEN:-}}}"
: "${TOKEN:?falta GITHUB_TOKEN (basta uno de solo lectura de Actions)}"
for b in curl jq awk; do
  command -v "$b" >/dev/null || { echo "falta $b" >&2; exit 1; }
done

# El token va por `--config -` y no por `-H`: en argv lo lee cualquiera con un
# `ps` en la misma máquina, y esto se corre tanto en el portátil como en el
# piloto.
api() {
  printf 'header = "Authorization: Bearer %s"\nheader = "Accept: application/vnd.github+json"\n' "$TOKEN" |
    curl -fsSL --max-time 60 --config - "$API/repos/$REPO/$1"
}

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# 1. LO DECLARADO, por (flujo, job). Un job se identifica por su `name:`; si no
#    lo tiene, por su clave, que es lo que enseña la forja en ese caso. La
#    pareja con el fichero evita confundir dos jobs que se llamen igual en
#    flujos distintos —`lint` hay varios— y darle a uno el reloj del otro.
for wf in "$RAIZ"/.github/workflows/*.yml; do
  [ -f "$wf" ] || continue
  awk -v flujo="$(basename "$wf")" '
    function volcar() { if (clave != "") print flujo "\t" (nombre != "" ? nombre : clave) "\t" (reloj != "" ? reloj : "-") }
    /^jobs:[[:space:]]*$/ { enjobs = 1; next }
    !enjobs { next }
    /^[^[:space:]#]/ { volcar(); clave = ""; enjobs = 0; next }
    /^  [A-Za-z0-9_-]+:[[:space:]]*$/ { volcar(); clave = $1; sub(/:$/, "", clave); nombre = ""; reloj = ""; next }
    /^    name:[[:space:]]/ { nombre = $0; sub(/^    name:[[:space:]]*/, "", nombre); gsub(/^["'"'"']|["'"'"']$/, "", nombre); next }
    /^    timeout-minutes:[[:space:]]/ { reloj = $0; sub(/^    timeout-minutes:[[:space:]]*/, "", reloj); next }
    END { volcar() }
  ' "$wf"
done > "$tmp/declarado"

[ -s "$tmp/declarado" ] || { echo "not ok - no he sabido leer ningún job de .github/workflows/" >&2; exit 1; }

# 2. LO MEDIDO. Solo tareas en verde: lo que tardó una cortada o fallida no dice
#    cuánto dura el trabajo, dice cuándo se rindió. Y del mismo job nos quedamos
#    con el PEOR caso: el reloj tiene que aguantar la vez que más tardó, no la
#    media.
echo "midiendo los últimos $TAREAS runs en verde de $REPO…"
# GitHub no lista tareas sueltas: se piden los runs en verde (de más nuevo a más
# viejo, 100 por página como mucho) y de cada uno sus jobs. El fichero del
# workflow sale de `path` y hace el papel del `workflow_id` de la forja.
por_pagina="$TAREAS"; [ "$por_pagina" -le 100 ] || por_pagina=100
api "actions/runs?status=success&per_page=$por_pagina" |
  jq -r --argjson n "$TAREAS" '[.workflow_runs[]][:$n][]
         | "\(.id)\t\(.path // "" | split("/") | last)\t\(.run_number)"' \
  > "$tmp/runs" || { echo "not ok - GitHub no ha contestado" >&2; exit 1; }
: > "$tmp/tareas"
while IFS=$'\t' read -r id flujo numero; do
  [ -n "$id" ] || continue
  api "actions/runs/$id/jobs?per_page=100" |
    jq -r --arg f "$flujo" --arg n "$numero" '.jobs[] | select(.conclusion == "success")
           | [$f, .name, .started_at, .completed_at, $n] | @tsv' >> "$tmp/tareas" || true
done < "$tmp/runs"

[ -s "$tmp/tareas" ] || { echo "not ok - ninguna tarea en verde entre las últimas $TAREAS" >&2; exit 1; }

# La resta se hace a mano sobre HH:MM:SS y no con `date -d` ni con el `mktime`
# de awk: ninguno de los dos existe a la vez en macOS y en el mawk de Ubuntu.
# Una tarea dura menos de 24 h —el `runner.timeout` del host son 3—, así que el
# salto de medianoche se corrige comparando la FECHA, y los dos extremos llevan
# el mismo huso.
#
# Y una diferencia negativa DENTRO del mismo día se descarta en vez de tratarla
# como un salto de día: hay tareas con `updated_at` unos segundos por delante de
# `created_at`, y darlas por buenas las convertía en 1439 minutos que se comían
# el máximo de su job y ponían en rojo medio CI.
awk -F'\t' '
  function s(t,   h) { h = substr(t, 12, 8); split(h, p, ":"); return p[1] * 3600 + p[2] * 60 + p[3] }
  { d = s($4) - s($3)
    if (d < 0) { if (substr($3, 1, 10) == substr($4, 1, 10)) next; d += 86400 }
    if (d > peor[$1 "\t" $2]) { peor[$1 "\t" $2] = d; run[$1 "\t" $2] = $5 } }
  END { for (k in peor) print k "\t" peor[k] "\t" run[k] }
' "$tmp/tareas" | sort -t"$(printf '\t')" -k3,3nr > "$tmp/peor"

# 3. EL VEREDICTO.
printf '\n%-52s %8s %8s %7s\n' "job" "medido" "reloj" "uso"
awk -F'\t' -v margen="$MARGEN" -v decl="$tmp/declarado" '
  # Un job de matriz se llama `trivy image (${{ matrix.grupo }})` en el YAML y
  # `trivy image (cloud)` en la forja: sin convertirlo en patrón, la mitad del
  # CI salía como «ese job ya no está» y el guardián no vigilaba nada de lo que
  # más tarda. La expresión se escapa entera y solo la interpolación es comodín.
  function patron(n,   p) {
    p = n
    gsub(/\$\{\{[^}]*\}\}/, "\001", p)
    gsub(/[][(){}.*+?^$|\\]/, "\\\\&", p)
    gsub(/\001/, ".*", p)
    return "^" p "$"
  }
  BEGIN {
    FS = "\t"
    while ((getline linea < decl) > 0) {
      split(linea, c, "\t")
      reloj[c[1] "\t" c[2]] = c[3]
      if (c[2] ~ /\$\{\{/) { i = ++npat[c[1]]; pat[c[1], i] = patron(c[2]); patreloj[c[1], i] = c[3] }
    }
  }
  {
    clave = $1 "\t" $2; seg = $3; run = $4
    corto = substr($2, 1, 50)
    r = (clave in reloj) ? reloj[clave] : ""
    if (r == "") for (i = 1; i <= npat[$1]; i++) if ($2 ~ pat[$1, i]) { r = patreloj[$1, i]; break }
    # Un job renombrado o retirado se anota pero NO pone esto en rojo: su
    # nombre viejo sigue en el historial y saldría durante horas sin que haya
    # nada que arreglar. Se va solo en cuanto la ventana avanza.
    if (r == "") { printf "%-52s %6dm%02d %8s %7s  ← ese job ya no está en %s\n", corto, seg / 60, seg % 60, "?", "?", $1; next }
    if (r == "-") { printf "%-52s %6dm%02d %8s %7s  ← CORRIÓ SIN RELOJ (%s)\n", corto, seg / 60, seg % 60, "ninguno", "inf", $1; rojos++; next }
    uso = seg * 100 / (r * 60)
    aviso = ""
    if (seg * margen > r * 60) { aviso = sprintf("  ← SIN MARGEN: sube timeout-minutes a %d en %s (run %s)", int((seg * margen + 59) / 60), $1, run); rojos++ }
    printf "%-52s %6dm%02d %7dm %6d%%%s\n", corto, seg / 60, seg % 60, r, uso, aviso
  }
  END { exit (rojos > 0 ? 1 : 0) }
' "$tmp/peor"
veredicto=$?

echo
if [ "$veredicto" -eq 0 ]; then
  echo "ok - todo job medido tiene un reloj de al menos ${MARGEN}× lo que tarda"
  exit 0
fi
echo "not ok - hay jobs sin el margen de ${MARGEN}×: el próximo que crezca muere por timeout" >&2
exit 1
