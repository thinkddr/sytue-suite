#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Recupera sólo la marca de un run que ya verificó todas sus imágenes.

Usa la sesión existente de gh; nunca extrae ni guarda su credencial. Por defecto
sólo muestra el manifiesto. --publicar crea una etiqueta anotada y su referencia,
sin reconstruir imágenes ni lanzar los clientes de escritorio. No mueve marcas.
"""

import argparse
import json
import re
import subprocess
import sys
from urllib.parse import quote

REPO = "thinkddr/sytue-one"
CIERRE = "Comprobar lo publicado y sus digest"
MARCA = "Firmar y marcar como desplegable"
ESCRITORIO = "Encadenar release de escritorio"


def api(path, data=None, *, missing=False, raw=False):
    command = ["gh", "api", f"repos/{REPO}/{path}"]
    if data is not None:
        command += ["--method", "POST", "--input", "-"]
    result = subprocess.run(
        command, input=None if data is None else json.dumps(data),
        capture_output=True, text=True, check=False,
    )
    # gh reciente protege las secuencias ANSI de los logs. No se muestran: se
    # analiza únicamente el manifiesto de texto conocido. Versiones anteriores
    # no tienen esta opción, así que sólo se usa cuando gh la solicita.
    if result.returncode and raw and "pass --allow-escape-sequences" in result.stderr:
        result = subprocess.run(command + ["--allow-escape-sequences"],
                                capture_output=True, text=True, check=False)
    if result.returncode:
        if missing and "HTTP 404" in result.stderr:
            return None
        raise ValueError(f"GitHub rechazó {path.split('?')[0]}; no se publica la marca")
    return result.stdout if raw else json.loads(result.stdout)


def cuerpo(log, version, perfil, commit, registro):
    lines = [re.sub(r"^\d{4}-\d\d-\d\dT\S+Z ", "", line)
             for line in log.splitlines()]
    starts = [i for i, line in enumerate(lines) if line == "sytue-desplegable 1"]
    if len(starts) != 1:
        raise ValueError("el job no contiene un único manifiesto de publicación")
    start = starts[0]
    header = ["sytue-desplegable 1", f"version: {version}", f"perfil: {perfil}",
              f"commit: {commit}", f"registro: {registro}"]
    if lines[start:start + 5] != header:
        raise ValueError("el manifiesto no corresponde al tag, commit, perfil o registro")
    digests = []
    for line in lines[start + 5:]:
        if not re.fullmatch(r"sha256:[0-9a-f]{64}  [a-z0-9-]+:" + re.escape(version), line):
            break
        digests.append(line)
    names = [line.split("  ")[1] for line in digests]
    # El cierre verde ya exigió igualdad con arquitecturas-imagen.sh de ESTE
    # commit. Aquí no se mantiene un segundo catálogo ni se inventan digests.
    counts = [int(match.group(1)) for line in lines
              if (match := re.fullmatch(r"✓ las (\d+) imágenes de " + re.escape(version)
                                       + r" arrancan en su destino", line))]
    if (len(counts) != 1 or counts[0] != len(names) or len(names) < 20
            or names != sorted(set(names))):
        raise ValueError("el manifiesto está incompleto, desordenado o duplica imágenes")
    return "\n".join(header + digests) + "\n"


def validar_jobs(jobs):
    cierres = [job for job in jobs if job["name"] == CIERRE]
    marcas = [job for job in jobs if job["name"] == MARCA]
    turnos = [job for job in jobs if job["name"].startswith("Construir y publicar (turno ")]
    if (len(cierres) != 1 or cierres[0]["conclusion"] != "success"
            or len(marcas) != 1 or marcas[0]["conclusion"] != "failure"
            or len(turnos) != 3 or any(job["conclusion"] != "success" for job in turnos)):
        raise ValueError("sólo se recupera una marca fallida tras tres turnos y cierre verdes")
    if any(job["conclusion"] != "success" and job["name"] not in (MARCA, ESCRITORIO)
           for job in jobs):
        raise ValueError("hay otro job fallido o incompleto; no se publica")
    return cierres[0]["id"]


def resolver(ref):
    obj = ref["object"]
    for _ in range(5):
        if obj["type"] == "commit":
            return obj["sha"]
        if obj["type"] != "tag":
            break
        obj = api(f"git/tags/{obj['sha']}")["object"]
    raise ValueError("la referencia no resuelve a un commit")


def comprobar_marca(ref, manifest, commit):
    if ref["object"]["type"] != "tag":
        raise ValueError("la marca existente no es anotada; no se mueve")
    tag = api(f"git/tags/{ref['object']['sha']}")
    if tag["object"]["type"] != "commit" or tag["object"]["sha"] != commit:
        raise ValueError("la marca existente apunta a otro commit; no se mueve")
    if tag["message"].rstrip("\n") != manifest.rstrip("\n"):
        raise ValueError("la marca existente contiene otro manifiesto; no se mueve")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run", required=True, type=int)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--perfil", choices=("piloto", "completo"), default="completo")
    parser.add_argument("--registro", default="ghcr.io/thinkddr")
    parser.add_argument("--publicar", action="store_true")
    args = parser.parse_args()
    if not re.fullmatch(r"v\d+\.\d+\.\d+", args.tag) or args.run <= 0:
        raise ValueError("se exige un run positivo y un tag vX.Y.Z")
    if args.registro != "ghcr.io/thinkddr":
        raise ValueError("esta recuperación sólo admite el registro de producción")
    run = api(f"actions/runs/{args.run}")
    if (run["status"] != "completed" or run["conclusion"] != "failure"
            or run["path"] != ".github/workflows/imagenes-produccion.yml"
            or run["head_repository"]["full_name"] != REPO):
        raise ValueError("el run no es una publicación fallida de este repositorio")
    commit = run["head_sha"]
    tag_ref = api(f"git/ref/tags/{quote(args.tag, safe='')}")
    if resolver(tag_ref) != commit:
        raise ValueError("el tag ya no apunta al commit que construyó el run")
    jobs = api(f"actions/runs/{args.run}/attempts/{run['run_attempt']}/jobs?per_page=100")
    if jobs["total_count"] != len(jobs["jobs"]):
        raise ValueError("no se recibieron todos los jobs")
    cierre = validar_jobs(jobs["jobs"])
    log = api(f"actions/jobs/{cierre}/logs", raw=True)
    manifest = cuerpo(log, args.tag, args.perfil, commit, args.registro)
    # Con firma habilitada se recupera por el entorno protegido, nunca se baja
    # silenciosamente a una etiqueta sin firma. Se mira también main por si la
    # clave se activó después de cortar este tag.
    for ref in (commit, "main"):
        if api(f"contents/deploy/firma/publicacion.pub?ref={ref}", missing=True) is not None:
            raise ValueError("la firma está activada: requiere recuperación en el entorno publicacion")
    marca = f"desplegable/{args.perfil}/{args.tag}"
    ref_path = f"git/ref/tags/{marca}"
    existing = api(ref_path, missing=True)
    if existing is not None:
        comprobar_marca(existing, manifest, commit)
        print(f"La marca {marca} ya contiene el manifiesto verificado; sin cambios.")
        return
    if not args.publicar:
        print(manifest, end="")
        print("Validado. Añade --publicar para crear sólo la marca.", file=sys.stderr)
        return
    annotated = api("git/tags", {"tag": marca, "message": manifest,
                                  "object": commit, "type": "commit"})
    api("git/refs", {"ref": f"refs/tags/{marca}", "sha": annotated["sha"]})
    comprobar_marca(api(ref_path), manifest, commit)
    print(f"Publicada y verificada {marca}: {commit}, {len(manifest.splitlines()) - 5} imágenes.")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, json.JSONDecodeError, OSError) as error:
        print(f"Error: {error}", file=sys.stderr)
        sys.exit(1)
