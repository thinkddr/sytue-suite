#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Consulta o registra los IDs y capacidades de las extensiones WidgetKit.

No crea certificados, perfiles, grupos ni miembros. Apple exige asociar el App
Group en Developer Portal; la firma debe comprobar después el perfil resultante.
Por defecto sólo hace GET y falla si falta el registro o alguna capacidad. Esto
no acredita la asociación al grupo: hay que comprobar el perfil resultante.
El modo --registrar es una acción explícita de operador.
"""
import argparse
import importlib.util
import json
import pathlib
import re
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
    attrs = capacidad.get("attributes")
    settings = attrs.get("settings") if isinstance(attrs, dict) else None
    if not isinstance(settings, list) or any(not isinstance(s, dict) for s in settings):
        return False
    niveles = [s for s in settings if s.get("key") == "DATA_PROTECTION_PERMISSION_LEVEL"]
    if len(niveles) != 1:
        return False
    opciones = niveles[0].get("options")
    if not isinstance(opciones, list) or not opciones or any(not isinstance(o, dict) for o in opciones):
        return False
    # GET puede proyectar sólo la opción seleccionada, sin el booleano opcional
    # del esquema de Apple. Aceptamos únicamente esta forma observada exacta;
    # POST/PATCH siguen enviando enabled:true y el CMS debe autorizar Complete.
    if len(opciones) == 1 and set(opciones[0]) == {"key"}:
        return opciones[0]["key"] == "COMPLETE_PROTECTION"
    if any(type(o.get("enabled")) is not bool for o in opciones):
        return False
    claves = [o.get("key") for o in opciones]
    if any(not isinstance(k, str) for k in claves) or len(set(claves)) != len(claves):
        return False
    habilitadas = [o.get("key") for o in opciones if o.get("enabled") is True]
    return habilitadas == ["COMPLETE_PROTECTION"]


def leer_capacidades(asc, extension, identificador):
    campos = urllib.parse.urlencode({"fields[bundleIdCapabilities]": "capabilityType,settings"})
    existentes = asc.lista("/v1/bundleIds/" + extension["id"] + "/bundleIdCapabilities?" + campos)
    capacidades = {}
    for capacidad in existentes:
        clave = capacidad.get("attributes", {}).get("capabilityType")
        if clave in capacidades:
            raise RuntimeError(identificador + ": capacidad repetida " + str(clave))
        capacidades[clave] = capacidad
    return capacidades


def diagnostico_proteccion(capacidades):
    """Sólo estructura y opciones de Data Protection; nunca IDs ni descripciones."""
    def clave(valor):
        return valor if isinstance(valor, str) and re.fullmatch(r"[A-Za-z][A-Za-z0-9_]{0,79}", valor) else "<" + type(valor).__name__ + ">"

    def claves(valor):
        return [clave(k) for k in list(valor)[:16]] if isinstance(valor, dict) else []

    def habilitada(valor):
        if valor is None or isinstance(valor, bool) or type(valor) is int and valor in (0, 1):
            return valor
        if isinstance(valor, str) and valor in ("true", "false", "0", "1"):
            return valor
        return "<" + type(valor).__name__ + ">"

    capacidad = capacidades.get("DATA_PROTECTION")
    resultado = {"present": capacidad is not None,
                 "observedCapabilityTypes": [clave(k) for k in list(capacidades)[:32]]}
    if not isinstance(capacidad, dict):
        return resultado
    attrs = capacidad.get("attributes")
    if not isinstance(attrs, dict):
        return {**resultado, "attributesType": type(attrs).__name__}
    settings = attrs.get("settings")
    resultado.update({"attributeKeys": claves(attrs), "settingsType": type(settings).__name__, "settings": []})
    for setting in settings[:8] if isinstance(settings, list) else []:
        if not isinstance(setting, dict):
            resultado["settings"].append({"type": type(setting).__name__})
            continue
        opciones = setting.get("options")
        dato = {"key": clave(setting.get("key")), "keys": claves(setting),
                "optionsType": type(opciones).__name__, "options": []}
        for opcion in opciones[:8] if isinstance(opciones, list) else []:
            if not isinstance(opcion, dict):
                dato["options"].append({"type": type(opcion).__name__})
                continue
            enabled = opcion.get("enabled")
            dato["options"].append({"key": clave(opcion.get("key")), "keys": claves(opcion),
                                    "enabled": habilitada(enabled), "enabledType": type(enabled).__name__})
        resultado["settings"].append(dato)
    return resultado


def registrar(asc, app, escribir, grupo):
    host = buscar(asc, app["id"])
    if not host:
        raise RuntimeError(app["clave"] + ": falta el App ID del host; no se crea automáticamente")
    plataforma = host["attributes"]["platform"]
    if plataforma not in ("IOS", "MAC_OS", "UNIVERSAL"):
        raise RuntimeError(app["clave"] + ": plataforma del host desconocida")
    identificador = app["id"] + ".widgets"
    extension = buscar(asc, identificador)
    mutado = False
    if extension and extension["attributes"]["platform"] != plataforma:
        raise RuntimeError(identificador + ": plataforma distinta del host")
    if not extension and escribir:
        attrs = {"identifier": identificador, "name": app["nombre"] + " Widgets", "platform": plataforma}
        if host["attributes"].get("seedId"):
            attrs["seedId"] = host["attributes"]["seedId"]
        extension = asc.escribir("bundleIds", attrs)["data"]
        mutado = True
    requeridas = {"APP_GROUPS": None}
    if plataforma in ("IOS", "UNIVERSAL"):
        requeridas["DATA_PROTECTION"] = PROTECCION
    capacidades = {}
    if extension:
        capacidades = leer_capacidades(asc, extension, identificador)
        for tipo, settings in requeridas.items():
            actual = capacidades.get(tipo)
            if actual and (settings is None or completa(actual)):
                continue
            if escribir:
                attrs = {"capabilityType": tipo}
                if settings is not None:
                    attrs["settings"] = settings
                if actual:
                    asc.escribir("bundleIdCapabilities", attrs, id=actual["id"])
                else:
                    asc.escribir("bundleIdCapabilities", attrs,
                                 rels={"bundleId": {"type": "bundleIds", "id": extension["id"]}})
                mutado = True
        if mutado:
            # Apple puede omitir settings en el POST/PATCH. Sólo el GET acredita
            # la configuración persistida, nunca el payload que hemos enviado.
            capacidades = leer_capacidades(asc, extension, identificador)
    disponibles = []
    for tipo, settings in requeridas.items():
        actual = capacidades.get(tipo)
        if actual and (settings is None or completa(actual)):
            disponibles.append(tipo)
    return {"app": app["clave"], "widget": identificador, "platform": plataforma,
            "registered": extension is not None, "capabilities": disponibles,
            "requiredCapabilities": list(requeridas), "group": grupo,
            "registrationReady": extension is not None and len(disponibles) == len(requeridas),
            "dataProtectionDiagnostic": diagnostico_proteccion(capacidades),
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
    incompletas = []
    for nombre in seleccion:
        try:
            resultado = registrar(asc, catalogo[nombre], opciones.registrar, grupo)
            print(json.dumps(resultado))
            if not resultado["registrationReady"]:
                incompletas.append(nombre)
        except (RuntimeError, KeyError) as error:
            print("::error::" + str(error), file=sys.stderr)
            fallos.append(nombre)
    if fallos:
        raise RuntimeError("sin terminar: " + " ".join(fallos))
    return int(bool(incompletas))


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (RuntimeError, KeyError, ValueError) as error:
        print("::error::Apple Widgets: " + str(error), file=sys.stderr)
        sys.exit(1)
