# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Qué errores de Poppler son NUEVOS en lo que escribe CertiPDF.

medir.py cuenta como mala toda salida en la que Poppler protesta, pero muchos
ficheros del corpus ya hacen protestar a Poppler de entrada (fuentes que no
existen, operadores inventados): esa queja no es de CertiPDF. Esto compara, por
fichero, los mensajes de la primera página de la entrada con los de cada salida
y deja en `salidas[op].poppler_nuevos` solo los que no estaban. informe.py los
usa cuando existen. Reescribe resultados.jsonl (después de la tanda final, que
es la que trae veraPDF).

Uso: poppler.py <dir de medida> [--hilos 3] [--claves claves.json]
"""
import argparse
import concurrent.futures
import json
import os
import re
import subprocess
import tempfile


def mensajes(ruta, clave=""):
    args = ["pdftoppm", "-r", "10", "-f", "1", "-l", "1", "-png"]
    if clave:
        args += ["-upw", clave]
    with tempfile.TemporaryDirectory() as tmp:
        try:
            p = subprocess.run(args + [ruta, os.path.join(tmp, "p")], capture_output=True, timeout=60)
            err = p.stderr.decode("utf-8", "replace")
        except subprocess.TimeoutExpired:
            return {"tiempo agotado"}
    # Sin posiciones ni números sueltos: lo que identifica el problema es el texto.
    return {re.sub(r"\(\d+\)|\b\d+\b", "#", l).strip() for l in err.splitlines()
            if re.search(r"Syntax Error|Internal Error|Error:", l)}


def uno(fila, claves):
    entrada = mensajes(fila["ruta"], claves.get(os.path.basename(fila["ruta"]), ""))
    fila["poppler_entrada"] = sorted(entrada)
    for s in fila.get("salidas", {}).values():
        if os.path.exists(s.get("ruta", "")):
            s["poppler_nuevos"] = sorted(mensajes(s["ruta"]) - entrada)
    return fila


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("medida")
    ap.add_argument("--hilos", type=int, default=3)
    ap.add_argument("--claves", default="")
    a = ap.parse_args()
    claves = json.load(open(a.claves)) if a.claves else {}
    with open(os.path.join(a.medida, "resultados.jsonl")) as f:
        filas = [json.loads(l) for l in f]
    with concurrent.futures.ThreadPoolExecutor(a.hilos) as ex:
        filas = list(ex.map(lambda fila: uno(fila, claves), filas))
    with open(os.path.join(a.medida, "resultados.jsonl"), "w") as f:
        for fila in sorted(filas, key=lambda x: x["id"]):
            f.write(json.dumps(fila, ensure_ascii=False) + "\n")
    print(len(filas), "ficheros")


if __name__ == "__main__":
    main()
