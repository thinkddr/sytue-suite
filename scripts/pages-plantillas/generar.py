#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Genera las plantillas de Pages en services/pages/internal/plantillas/<clave>.json.

    python3 scripts/pages-plantillas/generar.py [clave ...]

Sin argumentos genera todos los sectores de `sectores/`. El JSON es compacto y con las claves
ordenadas: el mismo fuente da siempre los mismos bytes.
"""

import importlib
import json
import pathlib
import sys

AQUI = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(AQUI))
DESTINO = AQUI.parents[1] / "services" / "pages" / "internal" / "plantillas"

import lib  # noqa: E402
import looks  # noqa: E402


def main(claves):
    malos = looks.comprobar()
    if malos:
        sys.exit("paletas sin AA:\n" + "\n".join(malos))
    todas = sorted(p.stem for p in (AQUI / "sectores").glob("*.py") if not p.name.startswith("_"))
    claves = claves or todas
    for k in claves:
        mod = importlib.import_module(f"sectores.{k}")
        pl = mod.build()
        problemas = lib.revisar(pl) + lib.validar_rapido(pl)
        if problemas:
            sys.exit(f"{k}: {problemas}")
        exento = pl.pop("exento", {})
        if exento:
            print(f"{k}: EXENTO (repetir en catalogo_test.go): {exento}")
        if '"$inverse"' in json.dumps(pl):
            c = pl["ir"]["theme"]["colors"]
            if lib.contraste(c["primary"], c["inverse"]) < 3:
                sys.exit(f"{k}: usa fondo oscuro y primary/inverse < 3")
        out = DESTINO / f"{pl['key']}.json"
        out.write_text(json.dumps(pl, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n", encoding="utf-8")
        print(f"{pl['key']}: {out.stat().st_size // 1024} KB")


if __name__ == "__main__":
    main(sys.argv[1:])
