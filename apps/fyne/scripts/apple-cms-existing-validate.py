#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Valida los 93 CMS existentes por GET: no firma, no instala y no usa llaveros."""
import importlib.util
import json
import os
import pathlib
import re
import sys
import urllib.parse
import urllib.request

ORIGEN = "https://api.appstoreconnect.apple.com"
RUTAS = re.compile(r"/v1/(?:bundleIds(?:/[A-Za-z0-9-]+/(?:bundleIdCapabilities|profiles))?|profiles/[A-Za-z0-9-]+(?:/certificates)?)")
AQUI = pathlib.Path(__file__).resolve().parent


class SinRedirecciones(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise ValueError("el inventario Apple no sigue redirecciones")


def importar(nombre, ruta):
    spec = importlib.util.spec_from_file_location(nombre, ruta)
    modulo = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(modulo)
    return modulo


def cliente_solo_lectura(api):
    # urllib conserva Authorization al redirigir. Este proceso sólo consulta la
    # API Apple fijada: rechaza el salto antes de enviar una segunda petición.
    urllib.request.install_opener(urllib.request.build_opener(SinRedirecciones()))
    class SoloLectura(api.Apple):
        def pedir(self, metodo, ruta, cuerpo=None, tolerar=()):
            direccion = urllib.parse.urlsplit(ruta if ruta.startswith("https://") else ORIGEN + ruta)
            if (metodo != "GET" or cuerpo is not None or direccion.scheme != "https"
                    or direccion.netloc != "api.appstoreconnect.apple.com"
                    or direccion.username or direccion.password or direccion.fragment
                    or not RUTAS.fullmatch(direccion.path)):
                raise ValueError("se requiere una consulta GET de perfiles al origen Apple fijado")
            return super().pedir(metodo, ruta, tolerar=tolerar)
    return SoloLectura()


def validar_todos(perfiles, generador, asc):
    catalogo = {app["clave"]: app for app in generador.apps()}
    if len(catalogo) != 15:
        raise ValueError("el catálogo debe conservar las quince apps")
    observados = []
    for plataforma in perfiles.TIPOS:
        requisitos = [
            perfiles.requisitos(generador, app, plataforma, widget, archivos)
            for app in catalogo.values()
            for _, widget, archivos in perfiles.destinos_app(app)
        ]
        if len(requisitos) != 31 or len({r["id"] for r in requisitos}) != 31:
            raise ValueError("se requieren 31 destinos únicos por plataforma")
        for requisito in requisitos:
            resultado = perfiles.preparar(
                asc, requisito, plataforma, certificado="", salida=None, solo_existentes=True,
            )
            valido = resultado.get("profile_ready") is True and resultado.get("created") is False
            observados.append(valido)
            # Sólo identificadores del catálogo y el resultado, nunca CMS, API ni certificados.
            print(json.dumps({"app_id": requisito["id"], "platform": plataforma, "CMSValidated": valido}))
    completo = len(observados) == 93 and all(observados)
    print(json.dumps({"kind": "existing_cms_validation", "expected": 93,
                      "validated": sum(observados), "complete": completo,
                      "writesPerformed": 0, "keychainAccessed": False,
                      "signingIdentityChecked": False, "signingReady": False}))
    return 0 if completo else 1


def main():
    try:
        if os.environ.get("APPSTORE_API", ORIGEN) != ORIGEN:
            raise ValueError("no se admite otro origen Apple")
        if not all(os.environ.get(k) for k in (
            "APP_STORE_CONNECT_ISSUER_ID", "APP_STORE_CONNECT_KEY_IDENTIFIER", "APP_STORE_CONNECT_PRIVATE_KEY",
        )):
            raise ValueError("faltan credenciales de lectura Apple")
        sys.path.insert(0, str(AQUI))
        perfiles = importar("perfiles_cms_existentes", AQUI / "apple-perfiles.py")
        api = importar("api_cms_existentes", AQUI / "appstore-publicar.py")
        generador = perfiles.REGISTRO["importar"]("catalogo_cms_existentes", AQUI.parent / "xcode/generar.py")
        return validar_todos(perfiles, generador, cliente_solo_lectura(api))
    except (Exception, SystemExit):
        # También captura la salida controlada del firmante JWT sin volcar datos ajenos.
        print("::error::No se pudieron validar los 93 CMS existentes de Apple.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
