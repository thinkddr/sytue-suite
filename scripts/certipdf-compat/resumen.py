# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Una línea por fichero de un resultados.jsonl de medir.py, para mirarlo a ojo.

Uso: resumen.py resultados.jsonl [patrón]
"""
import json
import re
import sys

patron = sys.argv[2] if len(sys.argv) > 2 else ""
for linea in open(sys.argv[1]):
    f = json.loads(linea)
    if patron and not re.search(patron, f["id"]):
        continue
    c = f["clase"]
    print(f"{f['origen']}/{f['fichero']} [{c['productor']} {c['version']} cif={c.get('cifrado')} form={c['formulario']} "
          f"fuentes={','.join(c['fuentes'])} img={','.join(c['imagenes'])} firma={c.get('nivel_firma', '-')}] rc={f['go_rc']} {f.get('go_fallo', '')[:120]}")
    malas = {k: (v.get("error") or v.get("panico", ""))[:90] for k, v in f["ops"].items() if not v["ok"]}
    if malas:
        print("   ops que fallan:", malas)
    print("   pintado:", {k: (v["clase"], v.get("bloques")) for k, v in f["pintado"].items()}, "texto:", f.get("texto"))
    for op, s in f["salidas"].items():
        extra = {k: v for k, v in s.items() if k not in ("ruta", "qpdf", "pypdf", "poppler", "pierde")}
        print(f"   {op}: qpdf={s['qpdf']['clase']} pypdf={s['pypdf'][:50]} poppler={s['poppler']} pierde={s['pierde']} {extra}")
    if f.get("firmas_entrada_pyhanko"):
        print("   firmas de entrada, pyHanko:", f["firmas_entrada_pyhanko"])
        print("   firmas de entrada, CertiPDF:", (f["ops"].get("verificar", {}).get("datos") or {}).get("firmas"))
