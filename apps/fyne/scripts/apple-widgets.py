#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Consulta o registra los IDs y capacidades de las extensiones WidgetKit.

No crea certificados, perfiles, grupos ni miembros. Apple exige asociar el App
Group en Developer Portal; la firma debe comprobar después el perfil resultante.
Por defecto sólo hace GET. El modo --registrar es una acción explícita de operador.
"""
import argparse
import importlib.util
import json
import pathlib
import sys
import urllib.parse


AQUI = pathlib.Path(__file__).resolve().parent


def importar(nombre, ruta):
    spec = importlib.util.spec_from_file_location(nombre, ruta)
    modulo = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(modulo)
    return modulo


def buscar(asc, identificador):
    resultados = asc.lista("/v1/bundleIds?" + urllib.parse.urlencode({"filter[identifier]": identificador}))
    encontrados = [r for r in resultados if r.get("attributes", {}).get("identifier") == identificador]
    if len(encontrados) > 1:
        raise RuntimeError(identificador + ": Apple devuelve más de un App ID exacto")
    return encontrados[0] if encontrados else None


PROTECCION = [{"key": "DATA_PROTECTION_PERMISSION_LEVEL", "options": [{"key": "COMPLETE_PROTECTION", "enabled": True}]}]


def completa(capacidad):
    settings = capacidad.get("attributes", {}).get("settings") or []
    niveles = [s for s in settings if s.get("key") == "DATA_PROTECTION_PERMISSION_LEVEL"]
    if len(niveles) != 1:
        return False
    opciones = niveles[0].get("options") or []
    habilitadas = [o.get("key") for o in opciones if o.get("enabled") is True]
    return habilitadas == ["COMPLETE_PROTECTION"]


def registrar(asc, app, escribir, grupo):
    host = buscar(asc, app["id"])
    if not host:
        raise RuntimeError(app["clave"] + ": falta el App ID del host; no se crea automáticamente")
    plataforma = host["attributes"]["platform"]
    if plataforma not in ("IOS", "MAC_OS", "UNIVERSAL"):
        raise RuntimeError(app["clave"] + ": plataforma del host desconocida")
    identificador = app["id"] + ".widgets"
    extension = buscar(asc, identificador)
    if extension and extension["attributes"]["platform"] != plataforma:
        raise RuntimeError(identificador + ": plataforma distinta del host")
    if not extension and escribir:
        attrs = {"identifier": identificador, "name": app["nombre"] + " Widgets", "platform": plataforma}
        if host["attributes"].get("seedId"):
            attrs["seedId"] = host["attributes"]["seedId"]
        extension = asc.escribir("bundleIds", attrs)["data"]
    requeridas = {"APP_GROUPS": None}
    if plataforma in ("IOS", "UNIVERSAL"):
        requeridas["DATA_PROTECTION"] = PROTECCION
    capacidades = {}
    if extension:
        existentes = asc.lista("/v1/bundleIds/" + extension["id"] + "/bundleIdCapabilities")
        for capacidad in existentes:
            clave = capacidad.get("attributes", {}).get("capabilityType")
            if clave in capacidades:
                raise RuntimeError(identificador + ": capacidad repetida " + clave)
            capacidades[clave] = capacidad
        for tipo, settings in requeridas.items():
            actual = capacidades.get(tipo)
            if actual and (settings is None or completa(actual)):
                continue
            if escribir:
                attrs = {"capabilityType": tipo}
                if settings is not None:
                    attrs["settings"] = settings
                if actual:
                    capacidad = asc.escribir("bundleIdCapabilities", attrs, id=actual["id"])["data"]
                else:
                    capacidad = asc.escribir("bundleIdCapabilities", attrs,
                                             rels={"bundleId": {"type": "bundleIds", "id": extension["id"]}})["data"]
                capacidades[tipo] = capacidad
    disponibles = []
    for tipo, settings in requeridas.items():
        actual = capacidades.get(tipo)
        if actual and (settings is None or completa(actual)):
            disponibles.append(tipo)
    return {"app": app["clave"], "widget": identificador, "platform": plataforma,
            "registered": extension is not None, "capabilities": disponibles,
            "requiredCapabilities": list(requeridas), "group": grupo,
            "groupAssociationVerified": False, "profileRequired": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--registrar", action="store_true")
    parser.add_argument("--seco", action="store_true", help="validar el catálogo sin llamar a Apple")
    parser.add_argument("apps", nargs="*")
    opciones = parser.parse_args()
    if opciones.registrar and opciones.seco:
        parser.error("--seco y --registrar son incompatibles")
    generador = importar("sytue_xcode_widgets", AQUI.parent / "xcode/generar.py")
    catalogo = {a["clave"]: a for a in generador.apps() if a["widget_acciones"]}
    seleccion = opciones.apps or list(catalogo)
    if len(set(seleccion)) != len(seleccion) or any(a not in catalogo for a in seleccion):
        parser.error("las apps deben pertenecer al catálogo WidgetKit y no repetirse")
    grupo = generador.grupo_widgets()
    if opciones.seco:
        print(json.dumps({"widgets": [catalogo[a]["id"] + ".widgets" for a in seleccion], "group": grupo}))
        return
    cliente = importar("sytue_asc_widgets", AQUI / "appstore-publicar.py")
    asc = cliente.Apple()
    fallos = []
    for nombre in seleccion:
        try:
            print(json.dumps(registrar(asc, catalogo[nombre], opciones.registrar, grupo)))
        except (RuntimeError, KeyError) as error:
            print("::error::" + str(error), file=sys.stderr)
            fallos.append(nombre)
    if fallos:
        raise RuntimeError("sin terminar: " + " ".join(fallos))


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, KeyError, ValueError) as error:
        print("::error::Apple Widgets: " + str(error), file=sys.stderr)
        sys.exit(1)
