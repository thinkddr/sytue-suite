#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Doble EXCLUSIVO de tests de semántica; no forma parte del flujo de firma.

Los fixtures heredados son plist sin firma. Se sustituye únicamente el límite
CMS dentro del proceso de prueba, igual que el antiguo doble de security cms.
La firma/cadena/rol se ejercitan con CMS reales en apple_perfil-test.py.
No hay flag, variable de entorno ni modo de bypass en el código de producto.
"""
import importlib.util
import pathlib
import plistlib
import sys


def main():
    ruta = pathlib.Path(sys.argv[1])
    if ruta.name != "apple_perfil.py":
        return 2
    spec = importlib.util.spec_from_file_location("perfil_semantica_fixture", ruta)
    modulo = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(modulo)
    modulo.desde_archivo = lambda archivo, plataforma: plistlib.loads(pathlib.Path(archivo).read_bytes())
    return modulo.main(sys.argv[2:])


if __name__ == "__main__":
    sys.exit(main())
