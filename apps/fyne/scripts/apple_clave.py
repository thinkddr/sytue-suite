#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Clave de la API de App Store Connect (secreto de GitHub Actions), compartida por API y notarización."""
import base64
import binascii
import os
import re
import sys


def normalizar_clave(valor):
    """Contenido o referencia @file:/@env: de codemagic-cli-tools; nunca muestra valores ni rutas."""
    # El contrato de PrivateKeyArgument permite @file:/@env:, no solo el PEM:
    # github.com/codemagic-ci-cd/cli-tools/blob/master/docs/app-store-connect/README.md
    vistas = set()
    for _ in range(8):
        referencia = valor.strip().lstrip("\ufeff")
        if not referencia.startswith(("@file:", "@env:")):
            break
        if referencia in vistas:
            raise ValueError("referencia circular de clave")
        vistas.add(referencia)
        if referencia.startswith("@env:"):
            nombre = referencia[5:]
            if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", nombre) or not os.environ.get(nombre):
                raise ValueError("referencia @env sin una variable válida")
            valor = os.environ[nombre]
        else:
            try:
                with open(os.path.expanduser(referencia[6:]), encoding="utf-8") as f:
                    valor = f.read(65537)
                if len(valor) > 65536:
                    raise ValueError("archivo de clave demasiado grande")
            except (OSError, UnicodeError, ValueError):
                raise ValueError("referencia @file sin un archivo de clave legible") from None
    else:
        raise ValueError("demasiadas referencias de clave")

    def saltos(texto):
        return texto.strip().lstrip("\ufeff").replace("\\r\\n", "\n").replace("\\n", "\n").replace("\r\n", "\n").strip()

    texto = saltos(valor)
    if not texto.startswith("-----BEGIN "):
        try:
            contenido = base64.b64decode("".join(texto.split()), validate=True)
            # El cuerpo base64 de un .p8 también es una copia válida de su DER.
            if contenido.startswith(b"\x30"):
                tipo = "PRIVATE KEY"
                texto = f"-----BEGIN {tipo}-----\n" + base64.b64encode(contenido).decode() + f"\n-----END {tipo}-----"
            else:
                texto = saltos(contenido.decode("utf-8"))
        except (binascii.Error, ValueError, UnicodeError):
            raise ValueError("formato de clave no reconocido") from None
    bloque = re.fullmatch(r"-----BEGIN (PRIVATE KEY|EC PRIVATE KEY)-----\s+([A-Za-z0-9+/=\s]+)\s+-----END \1-----", texto)
    if bloque is None:
        raise ValueError("formato de clave no reconocido")
    try:
        contenido = base64.b64decode("".join(bloque[2].split()), validate=True)
    except (binascii.Error, ValueError):
        raise ValueError("cuerpo PEM no reconocido") from None
    cuerpo = base64.b64encode(contenido).decode()
    lineas = "\n".join(cuerpo[i : i + 64] for i in range(0, len(cuerpo), 64))
    return f"-----BEGIN {bloque[1]}-----\n{lineas}\n-----END {bloque[1]}-----\n"



def guardar(ruta):
    """Materializa un PEM nuevo con permisos 600; nunca imprime la credencial."""
    try:
        clave = normalizar_clave(os.environ["APP_STORE_CONNECT_PRIVATE_KEY"])
        fd = os.open(ruta, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as archivo:
            archivo.write(clave)
    except (KeyError, ValueError, OSError):
        print("::error::APP_STORE_CONNECT_PRIVATE_KEY: no se pudo preparar el PEM de la integración para notarytool", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("uso: apple_clave.py <fichero nuevo>")
    sys.exit(guardar(sys.argv[1]))
