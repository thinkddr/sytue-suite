#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# El trinquete de las capturas RANCIAS de `test/e2e/shots/`.
#
# Una captura anterior al último cambio de la paleta MIENTE EN EL COLOR, y no lo
# advierte nadie: es un PNG, se abre, se ve bonito y se saca una conclusión. Ya
# pasó: un agente concluyó de una captura del 2026-09-04 que `tema.Color("primary")`
# estaba mal porque su PNG pintaba NARANJA donde el token da tinta. El token era
# correcto; la hoja sobrescribe `--sytue-primary: var(--sytue-accent-800)` a
# propósito, y el rediseño «Tinta y papel» que lo hizo es del 2026-09-15. La captura
# tenía once días de menos y toda la razón aparente.
#
# Qué compara: la fecha del ÚLTIMO COMMIT de cada PNG contra la del último commit de
# la paleta. No la mtime del fichero, que un `git clone` pone a la de hoy y dejaría
# el gate en verde siempre.
#
# Es un TRINQUETE y no un «arréglalo ya»: la lista de las que hoy se sabe que están
# rancias vive en `test/e2e/shots/RANCIAS.txt` y se compara POR IGUALDAD. Así:
#
#   - una captura nueva que nazca rancia, o una que se quede rancia porque alguien
#     toca la paleta, pone esto en rojo;
#   - y una que se regenere y nadie borre de la lista, también — que es como una
#     lista de excepciones se convierte en un sitio donde esconder cosas.
#
# Uso: scripts/capturas-rancias.sh
set -euo pipefail

# Todo el guion ordena y compara en C. `sort` y `comm` tienen que usar la MISMA
# collation o comm da basura en silencio: en `en_US.UTF-8` la puntuación no pesa en el
# primer nivel y `docs-editor.png` y `docs-editor-presentation.png` salen al revés que
# en C, así que comm se cree que la entrada no está ordenada y no dice nada útil.
export LC_ALL=C

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PALETA="branding/tokens/colors.css"
SHOTS="test/e2e/shots"
LISTA="$SHOTS/RANCIAS.txt"

# Sin `.git` esto NO se salta: se cae. Saltarse en silencio es exactamente el fallo
# de `lint:marca`, que sin ImageMagick se autoexcusa y por eso no corre en el CI —un
# gate que sale verde sin haber mirado nada es peor que no tener gate—.
if ! git -C "$RAIZ" rev-parse --git-dir >/dev/null 2>&1; then
    echo "capturas-rancias: esto es un árbol SIN .git y este gate necesita el historial." >&2
    echo "  Compara la fecha del último commit de cada PNG con la de $PALETA; sin" >&2
    echo "  historial no hay nada que comparar, y saltárselo dejaría el gate en verde" >&2
    echo "  sin haber mirado una sola captura. Clona con historial o no lo lances." >&2
    exit 1
fi

fecha_de() { git -C "$RAIZ" log -1 --format=%ct -- "$1"; }

PALETA_TS="$(fecha_de "$PALETA")"
if [ -z "$PALETA_TS" ]; then
    echo "capturas-rancias: $PALETA no tiene commits: ¿se ha movido la paleta?" >&2
    exit 1
fi

rancias=""
vistas=0
for png in "$RAIZ/$SHOTS"/*.png; do
    [ -e "$png" ] || continue
    vistas=$((vistas + 1))
    rel="$SHOTS/$(basename "$png")"
    ts="$(fecha_de "$rel")"
    # Una captura sin commits es nueva y sin registrar: se trata como fresca, que es
    # lo que es, y si su autor no la comitea no hay gate que lo arregle.
    [ -n "$ts" ] || continue
    if [ "$ts" -lt "$PALETA_TS" ]; then
        rancias="$rancias$(basename "$png")
"
    fi
done

if [ "$vistas" -eq 0 ]; then
    echo "capturas-rancias: no hay ni un PNG en $SHOTS: el gate está mirando donde no es." >&2
    exit 1
fi

actual="$(printf '%s' "$rancias" | sort)"
registrada="$(grep -v -e '^[[:space:]]*#' -e '^[[:space:]]*$' "$RAIZ/$LISTA" 2>/dev/null | sort || true)"

if [ "$actual" = "$registrada" ]; then
    n="$(printf '%s' "$actual" | grep -c . || true)"
    echo "✓ capturas-rancias: $vistas capturas, $n rancias, todas registradas."
    echo "  Recuerda: las rancias VALEN PARA GEOMETRÍA, NO PARA COLOR."
    exit 0
fi

echo "capturas-rancias: la lista de $LISTA ya no es la de verdad." >&2
echo >&2
echo "  La paleta ($PALETA) cambió por última vez el $(date -r "$PALETA_TS" '+%Y-%m-%d' 2>/dev/null || echo "$PALETA_TS")." >&2
echo "  Toda captura ANTERIOR a esa fecha VALE PARA GEOMETRÍA Y REPARTO, NO PARA COLOR." >&2
echo >&2
comm -23 <(printf '%s\n' "$actual") <(printf '%s\n' "$registrada") | grep . | while read -r f; do
    echo "  + $f — rancia y NO registrada: regenérala, o añádela a $LISTA." >&2
done
comm -13 <(printf '%s\n' "$actual") <(printf '%s\n' "$registrada") | grep . | while read -r f; do
    echo "  - $f — registrada y YA NO es rancia: bórrala de $LISTA." >&2
done
exit 1
