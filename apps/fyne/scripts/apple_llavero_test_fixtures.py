#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Boundary double for the legacy shell contracts; never imported by production."""
import importlib.util
import os
from pathlib import Path
import subprocess
import shutil
import sys

assert os.environ.get("SYTUE_PRUEBA_LLAVERO") == "si"
assert os.environ.get("ARGV") and os.environ.get("LLAVERO")
operation, *args = sys.argv[2:]
own = os.environ["LLAVERO"]
if operation == "ruta":
    if os.environ.get("LLAVERO_INSEGURO") == "si":
        raise SystemExit(1)
    print(own)
elif operation == "cli":
    if args[0] == "fetch-signing-files":
        source = Path(os.environ["SYTUE_PRUEBA_PERFILES_FUENTE"])
        destination = Path(os.environ["SYTUE_APPLE_JOB_DIR"]) / "perfiles-apple"
        destination.mkdir(mode=0o700, exist_ok=True)
        for pattern in ("*.mobileprovision", "*.provisionprofile"):
            for profile in source.glob(pattern):
                shutil.copyfile(profile, destination / profile.name)
    raise SystemExit(subprocess.run(["app-store-connect", *args]).returncode)
elif operation == "importar":
    with open(os.environ["ARGV"], "a") as output:
        output.write("llavero importar " + " ".join(args) + "\n")
elif operation == "publicos":
    if os.environ.get("LLAVERO_INSEGURO") == "si":
        raise SystemExit(1)
    source = Path(os.environ["RAIZ"]) / "apps/fyne/scripts/apple_llavero.py"
    spec = importlib.util.spec_from_file_location("owned_keychain_public_fixture", source)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    try:
        module.verificar_publicos([Path(p) for p in args])
    except module.ErrorLlavero as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1) from None
    for path in args:
        subprocess.run(["security", "add-certificates", "-k", own, path], check=True)
elif operation == "limpiar":
    with open(os.environ["ARGV"], "a") as output:
        output.write("llavero limpiar\n")
    raise SystemExit(1 if os.environ.get("LIMPIEZA_RECHAZADA") == "si" else 0)
else:
    raise SystemExit(64)
